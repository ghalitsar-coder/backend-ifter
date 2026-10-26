package auth

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

// A recently rotated token may be replayed by concurrent refresh requests.
// Fail those without revoking the family; reuse after the grace window is
// treated as a stolen token.
const refreshReuseGrace = 10 * time.Second

type Service struct {
	pool       *pgxpool.Pool
	queries    *db.Queries
	tokens     *TokenManager
	refreshTTL time.Duration
	inviteTTL  time.Duration
	resetTTL   time.Duration
	bcryptCost int
	log        *slog.Logger
}

func NewService(pool *pgxpool.Pool, tokens *TokenManager, cfg ServiceConfig, log *slog.Logger) *Service {
	return &Service{
		pool:       pool,
		queries:    db.New(pool),
		tokens:     tokens,
		refreshTTL: cfg.RefreshTokenTTL,
		inviteTTL:  cfg.InviteTokenTTL,
		resetTTL:   cfg.ResetTokenTTL,
		bcryptCost: cfg.BcryptCost,
		log:        log,
	}
}

func (s *Service) Login(ctx context.Context, login, password, requestID string) (Session, error) {
	login = strings.TrimSpace(login)
	if login == "" || password == "" || strings.IndexByte(login, 0) >= 0 {
		return Session{}, ErrInvalidCredentials
	}

	user, err := s.queries.GetUserByUsername(ctx, login)
	if database.IsNotFound(err) {
		user, err = s.queries.GetUserByEmail(ctx, login)
	}
	if err != nil {
		if database.IsNotFound(err) {
			checkPassword(dummyHash, password)
			return Session{}, ErrInvalidCredentials
		}
		return Session{}, fmt.Errorf("find user by login: %w", err)
	}
	if user.Status != "active" || !user.PasswordHash.Valid {
		checkPassword(dummyHash, password)
		return Session{}, ErrInvalidCredentials
	}
	if !checkPassword(user.PasswordHash.String, password) {
		return Session{}, ErrInvalidCredentials
	}

	roles, err := s.queries.ListUserRoles(ctx, user.ID)
	if err != nil {
		return Session{}, fmt.Errorf("list user roles: %w", err)
	}

	refreshToken, refreshHash, err := newOpaqueToken()
	if err != nil {
		return Session{}, err
	}
	refreshExpiresAt := time.Now().Add(s.refreshTTL)

	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		if _, err := q.CreateRefreshToken(ctx, db.CreateRefreshTokenParams{
			UserID:    user.ID,
			TokenHash: refreshHash,
			ExpiresAt: pgtype.Timestamptz{Time: refreshExpiresAt, Valid: true},
		}); err != nil {
			return fmt.Errorf("create refresh token: %w", err)
		}
		if err := q.UpdateLastLogin(ctx, user.ID); err != nil {
			return fmt.Errorf("update last login: %w", err)
		}
		if err := s.audit(ctx, q, user.ID, "auth.login", "user", user.ID, requestID); err != nil {
			return fmt.Errorf("write login audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return Session{}, err
	}

	accessToken, accessExpiresAt, err := s.tokens.Issue(formatUUID(user.ID), int(user.TokenVersion))
	if err != nil {
		return Session{}, err
	}
	return Session{
		AccessToken:      accessToken,
		AccessExpiresAt:  accessExpiresAt,
		RefreshToken:     refreshToken,
		RefreshExpiresAt: refreshExpiresAt,
		User:             toUser(user, roles),
	}, nil
}

func (s *Service) Refresh(ctx context.Context, rawToken, requestID string) (Session, error) {
	if rawToken == "" {
		return Session{}, ErrInvalidRefresh
	}

	var authErr error
	var user User
	var tokenVersion int
	var refreshToken string
	var refreshExpiresAt time.Time

	err := database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		old, err := q.GetRefreshTokenByHashForUpdate(ctx, hashOpaqueToken(rawToken))
		if err != nil {
			if database.IsNotFound(err) {
				authErr = ErrInvalidRefresh
				return nil
			}
			return fmt.Errorf("find refresh token: %w", err)
		}

		now := time.Now()
		if old.RevokedAt.Valid {
			if !old.ReplacedBy.Valid || now.Sub(old.RevokedAt.Time) <= refreshReuseGrace {
				authErr = ErrInvalidRefresh
				return nil
			}
			if err := q.RevokeAllUserRefreshTokens(ctx, old.UserID); err != nil {
				return fmt.Errorf("revoke refresh token family: %w", err)
			}
			if err := s.audit(ctx, q, old.UserID, "auth.refresh_reuse", "user", old.UserID, requestID); err != nil {
				return fmt.Errorf("write refresh reuse audit: %w", err)
			}
			authErr = ErrRefreshReuse
			return nil
		}
		if !old.ExpiresAt.Valid || !old.ExpiresAt.Time.After(now) {
			if err := q.RevokeRefreshToken(ctx, old.ID); err != nil {
				return fmt.Errorf("revoke expired refresh token: %w", err)
			}
			authErr = ErrInvalidRefresh
			return nil
		}

		currentUser, err := q.GetUserByID(ctx, old.UserID)
		if err != nil {
			if database.IsNotFound(err) {
				authErr = ErrInvalidRefresh
				return nil
			}
			return fmt.Errorf("find refresh token user: %w", err)
		}
		if currentUser.Status != "active" {
			if err := q.RevokeAllUserRefreshTokens(ctx, old.UserID); err != nil {
				return fmt.Errorf("revoke inactive user sessions: %w", err)
			}
			authErr = ErrInvalidRefresh
			return nil
		}

		roles, err := q.ListUserRoles(ctx, old.UserID)
		if err != nil {
			return fmt.Errorf("list refresh user roles: %w", err)
		}

		var refreshHash string
		refreshToken, refreshHash, err = newOpaqueToken()
		if err != nil {
			return err
		}
		refreshExpiresAt = now.Add(s.refreshTTL)
		created, err := q.CreateRefreshToken(ctx, db.CreateRefreshTokenParams{
			UserID:    old.UserID,
			TokenHash: refreshHash,
			ExpiresAt: pgtype.Timestamptz{Time: refreshExpiresAt, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("create rotated refresh token: %w", err)
		}
		if err := q.RotateRefreshToken(ctx, db.RotateRefreshTokenParams{
			ReplacedBy: created.ID,
			ID:         old.ID,
		}); err != nil {
			return fmt.Errorf("rotate refresh token: %w", err)
		}

		user = toUser(currentUser, roles)
		tokenVersion = int(currentUser.TokenVersion)
		return nil
	})
	if err != nil {
		return Session{}, err
	}
	if authErr != nil {
		return Session{}, authErr
	}

	accessToken, accessExpiresAt, err := s.tokens.Issue(user.ID, tokenVersion)
	if err != nil {
		return Session{}, err
	}
	return Session{
		AccessToken:      accessToken,
		AccessExpiresAt:  accessExpiresAt,
		RefreshToken:     refreshToken,
		RefreshExpiresAt: refreshExpiresAt,
		User:             user,
	}, nil
}

func (s *Service) Logout(ctx context.Context, rawToken, requestID string) error {
	if rawToken == "" {
		return nil
	}

	return database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		token, err := q.GetRefreshTokenByHashForUpdate(ctx, hashOpaqueToken(rawToken))
		if err != nil {
			if database.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("find logout token: %w", err)
		}
		if token.RevokedAt.Valid {
			return nil
		}
		if err := q.RevokeRefreshToken(ctx, token.ID); err != nil {
			return fmt.Errorf("revoke logout token: %w", err)
		}
		if err := s.audit(ctx, q, token.UserID, "auth.logout", "user", token.UserID, requestID); err != nil {
			return fmt.Errorf("write logout audit: %w", err)
		}
		return nil
	})
}

func (s *Service) Authenticate(ctx context.Context, accessToken string) (User, error) {
	subject, tokenVersion, err := s.tokens.Parse(accessToken)
	if err != nil {
		return User{}, ErrUnauthorized
	}
	userID, err := parseUUID(subject)
	if err != nil {
		return User{}, ErrUnauthorized
	}

	user, err := s.queries.GetUserByID(ctx, userID)
	if err != nil {
		if database.IsNotFound(err) {
			return User{}, ErrUnauthorized
		}
		return User{}, fmt.Errorf("find authenticated user: %w", err)
	}
	if user.Status != "active" || int(user.TokenVersion) != tokenVersion {
		return User{}, ErrUnauthorized
	}
	roles, err := s.queries.ListUserRoles(ctx, user.ID)
	if err != nil {
		return User{}, fmt.Errorf("list authenticated user roles: %w", err)
	}
	return toUser(user, roles), nil
}

func (s *Service) EnsureBootstrapAdmin(ctx context.Context, admin BootstrapAdmin) error {
	admin.Username = strings.TrimSpace(admin.Username)
	admin.Email = strings.TrimSpace(admin.Email)
	admin.FullName = strings.TrimSpace(admin.FullName)
	if admin.Username == "" {
		return nil
	}
	adminCount, err := s.queries.CountUsersWithRole(ctx, "admin")
	if err != nil {
		return fmt.Errorf("count administrators: %w", err)
	}
	if adminCount > 0 {
		return nil
	}

	passwordHash, err := hashPassword(admin.Password, s.bcryptCost)
	if err != nil {
		return err
	}
	if admin.FullName == "" {
		admin.FullName = "Administrator"
	}

	var createdUsername, createdID string
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		user, err := q.CreateUser(ctx, db.CreateUserParams{
			Username:     admin.Username,
			Email:        optionalText(admin.Email),
			PasswordHash: pgtype.Text{String: passwordHash, Valid: true},
			FullName:     admin.FullName,
			Status:       "active",
		})
		if err != nil {
			return fmt.Errorf("create bootstrap administrator: %w", err)
		}
		if err := q.AssignUserRole(ctx, db.AssignUserRoleParams{
			UserID:     user.ID,
			RoleCode:   "admin",
			AssignedBy: user.ID,
		}); err != nil {
			return fmt.Errorf("assign bootstrap administrator role: %w", err)
		}
		if err := s.audit(ctx, q, user.ID, "user.bootstrap_admin", "user", user.ID, ""); err != nil {
			return fmt.Errorf("write bootstrap administrator audit: %w", err)
		}
		createdID, createdUsername = formatUUID(user.ID), user.Username
		return nil
	})
	if err != nil {
		// A second instance may have won the race. Treat that as success.
		if database.IsUniqueViolation(err) {
			adminCount, countErr := s.queries.CountUsersWithRole(ctx, "admin")
			if countErr == nil && adminCount > 0 {
				return nil
			}
		}
		return err
	}
	s.log.Info("bootstrap_admin_created", "user_id", createdID, "username", createdUsername)
	return nil
}

func (s *Service) ListUsers(ctx context.Context, status string, limit, offset int32) ([]UserSummary, int64, error) {
	if limit < 1 || limit > 100 || offset < 0 {
		return nil, 0, ErrInvalidInput
	}
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "", "pending", "active", "inactive", "locked", "system":
	default:
		return nil, 0, ErrInvalidInput
	}
	filter := optionalText(status)
	users, err := s.queries.ListUsers(ctx, db.ListUsersParams{
		Status:     filter,
		PageOffset: offset,
		PageLimit:  limit,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list users: %w", err)
	}
	total, err := s.queries.CountUsers(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}

	result := make([]UserSummary, 0, len(users))
	for _, user := range users {
		summary := UserSummary{
			ID:        formatUUID(user.ID),
			Username:  user.Username,
			Email:     user.Email.String,
			FullName:  user.FullName,
			Status:    user.Status,
			CreatedAt: user.CreatedAt.Time,
		}
		if user.LastLoginAt.Valid {
			t := user.LastLoginAt.Time
			summary.LastLoginAt = &t
		}
		result = append(result, summary)
	}
	return result, total, nil
}

func (s *Service) audit(ctx context.Context, q *db.Queries, actorID pgtype.UUID, action, entityType string, entityID pgtype.UUID, requestID string) error {
	_, err := q.CreateAuditLog(ctx, db.CreateAuditLogParams{
		ActorUserID: actorID,
		Action:      action,
		EntityType:  entityType,
		EntityID:    entityID,
		RequestID:   optionalText(requestID),
	})
	return err
}

func toUserSummary(user db.User) UserSummary {
	summary := UserSummary{
		ID:        formatUUID(user.ID),
		Username:  user.Username,
		Email:     user.Email.String,
		FullName:  user.FullName,
		Status:    user.Status,
		CreatedAt: user.CreatedAt.Time,
	}
	if user.LastLoginAt.Valid {
		t := user.LastLoginAt.Time
		summary.LastLoginAt = &t
	}
	return summary
}

func toUser(user db.User, roles []db.Role) User {
	result := User{
		ID:       formatUUID(user.ID),
		Username: user.Username,
		Email:    user.Email.String,
		FullName: user.FullName,
		Roles:    make([]string, 0, len(roles)),
	}
	for _, role := range roles {
		result.Roles = append(result.Roles, role.Code)
	}
	return result
}

func formatUUID(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return uuid.UUID(id.Bytes).String()
}

func parseUUID(value string) (pgtype.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: id, Valid: true}, nil
}

func optionalText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}
