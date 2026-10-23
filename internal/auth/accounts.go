package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/mail"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

const (
	credentialPurposeInvite = "invite"
	credentialPurposeReset  = "reset"
	minPasswordLength       = 12
	maxPasswordLength       = 72
)

func (s *Service) CreateUser(ctx context.Context, actorID string, input CreateUserInput, requestID string) (UserAccount, CredentialIssue, error) {
	actorUUID, err := parseUUID(actorID)
	if err != nil {
		return UserAccount{}, CredentialIssue{}, ErrUnauthorized
	}
	input, err = normalizeUserInput(input)
	if err != nil {
		return UserAccount{}, CredentialIssue{}, err
	}
	roleCodes, err := s.validateRoleCodes(ctx, input.Roles)
	if err != nil {
		return UserAccount{}, CredentialIssue{}, err
	}

	rawToken, tokenHash, err := newOpaqueToken()
	if err != nil {
		return UserAccount{}, CredentialIssue{}, err
	}
	expiresAt := time.Now().Add(s.inviteTTL)

	var account UserAccount
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		user, err := q.CreateUser(ctx, db.CreateUserParams{
			Username:     input.Username,
			Email:        optionalText(input.Email),
			PasswordHash: pgtype.Text{},
			FullName:     input.FullName,
			Status:       "pending",
		})
		if err != nil {
			if database.IsUniqueViolation(err) {
				return ErrConflict
			}
			return fmt.Errorf("create user: %w", err)
		}
		for _, code := range roleCodes {
			if err := q.AssignUserRole(ctx, db.AssignUserRoleParams{
				UserID:     user.ID,
				RoleCode:   code,
				AssignedBy: actorUUID,
			}); err != nil {
				return fmt.Errorf("assign user role %q: %w", code, err)
			}
		}
		if _, err := q.CreateCredentialToken(ctx, db.CreateCredentialTokenParams{
			UserID:    user.ID,
			Purpose:   credentialPurposeInvite,
			TokenHash: tokenHash,
			ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
			CreatedBy: actorUUID,
		}); err != nil {
			return fmt.Errorf("create invite token: %w", err)
		}
		if err := s.auditData(ctx, q, actorUUID, "user.create", "user", user.ID, nil, userAuditData(user, roleCodes), requestID); err != nil {
			return fmt.Errorf("write user create audit: %w", err)
		}
		account = toUserAccount(user, roleCodes)
		return nil
	})
	if err != nil {
		return UserAccount{}, CredentialIssue{}, err
	}
	return account, CredentialIssue{Token: rawToken, Purpose: credentialPurposeInvite, ExpiresAt: expiresAt}, nil
}

func (s *Service) GetUser(ctx context.Context, userID string) (UserAccount, error) {
	id, err := parseUUID(userID)
	if err != nil {
		return UserAccount{}, ErrInvalidInput
	}
	user, err := s.queries.GetUserByID(ctx, id)
	if err != nil {
		if database.IsNotFound(err) {
			return UserAccount{}, ErrNotFound
		}
		return UserAccount{}, fmt.Errorf("find user: %w", err)
	}
	roles, err := s.queries.ListUserRoles(ctx, user.ID)
	if err != nil {
		return UserAccount{}, fmt.Errorf("list user roles: %w", err)
	}
	return toUserAccount(user, roleCodes(roles)), nil
}

func (s *Service) UpdateUserRoles(ctx context.Context, actorID, userID string, roles []string, requestID string) (UserAccount, error) {
	actorUUID, err := parseUUID(actorID)
	if err != nil {
		return UserAccount{}, ErrUnauthorized
	}
	userUUID, err := parseUUID(userID)
	if err != nil {
		return UserAccount{}, ErrInvalidInput
	}
	newCodes, err := s.validateRoleCodes(ctx, roles)
	if err != nil {
		return UserAccount{}, err
	}

	var account UserAccount
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		if err := q.LockAdminGuard(ctx); err != nil {
			return fmt.Errorf("lock administrator guard: %w", err)
		}
		user, err := q.GetUserByIDForUpdate(ctx, userUUID)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("find user for role update: %w", err)
		}
		if user.Status == "system" {
			return ErrForbidden
		}
		currentRoles, err := q.ListUserRoles(ctx, user.ID)
		if err != nil {
			return fmt.Errorf("list current user roles: %w", err)
		}
		currentCodes := roleCodes(currentRoles)
		if equalStrings(currentCodes, newCodes) {
			account = toUserAccount(user, currentCodes)
			return nil
		}
		if err := q.DeleteUserRoles(ctx, user.ID); err != nil {
			return fmt.Errorf("delete user roles: %w", err)
		}
		for _, code := range newCodes {
			if err := q.AssignUserRole(ctx, db.AssignUserRoleParams{
				UserID:     user.ID,
				RoleCode:   code,
				AssignedBy: actorUUID,
			}); err != nil {
				return fmt.Errorf("assign user role %q: %w", code, err)
			}
		}
		admins, err := q.CountActiveAdmins(ctx)
		if err != nil {
			return fmt.Errorf("count active administrators: %w", err)
		}
		if admins == 0 {
			return ErrLastAdmin
		}
		if err := s.auditData(ctx, q, actorUUID, "user.roles.update", "user", user.ID,
			map[string]any{"roles": currentCodes}, map[string]any{"roles": newCodes}, requestID); err != nil {
			return fmt.Errorf("write user roles audit: %w", err)
		}
		account = toUserAccount(user, newCodes)
		return nil
	})
	if err != nil {
		return UserAccount{}, err
	}
	return account, nil
}

func (s *Service) UpdateUserStatus(ctx context.Context, actorID, userID, status, requestID string) (UserAccount, error) {
	actorUUID, err := parseUUID(actorID)
	if err != nil {
		return UserAccount{}, ErrUnauthorized
	}
	userUUID, err := parseUUID(userID)
	if err != nil {
		return UserAccount{}, ErrInvalidInput
	}
	status = strings.ToLower(strings.TrimSpace(status))
	if status != "active" && status != "inactive" {
		return UserAccount{}, ErrInvalidInput
	}

	var account UserAccount
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		if err := q.LockAdminGuard(ctx); err != nil {
			return fmt.Errorf("lock administrator guard: %w", err)
		}
		user, err := q.GetUserByIDForUpdate(ctx, userUUID)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("find user for status update: %w", err)
		}
		if user.Status == "system" {
			return ErrForbidden
		}
		if !user.PasswordHash.Valid {
			return ErrUserNotReady
		}
		if user.Status == status {
			roles, err := q.ListUserRoles(ctx, user.ID)
			if err != nil {
				return fmt.Errorf("list user roles: %w", err)
			}
			account = toUserAccount(user, roleCodes(roles))
			return nil
		}

		updated, err := q.UpdateUserStatus(ctx, db.UpdateUserStatusParams{Status: status, ID: user.ID})
		if err != nil {
			return fmt.Errorf("update user status: %w", err)
		}
		if status == "inactive" {
			if err := q.RevokeAllUserRefreshTokens(ctx, user.ID); err != nil {
				return fmt.Errorf("revoke inactive user sessions: %w", err)
			}
		}
		admins, err := q.CountActiveAdmins(ctx)
		if err != nil {
			return fmt.Errorf("count active administrators: %w", err)
		}
		if admins == 0 {
			return ErrLastAdmin
		}
		if err := s.auditData(ctx, q, actorUUID, "user.status.update", "user", user.ID,
			map[string]any{"status": user.Status}, map[string]any{"status": updated.Status}, requestID); err != nil {
			return fmt.Errorf("write user status audit: %w", err)
		}
		roles, err := q.ListUserRoles(ctx, user.ID)
		if err != nil {
			return fmt.Errorf("list updated user roles: %w", err)
		}
		account = toUserAccount(updated, roleCodes(roles))
		return nil
	})
	if err != nil {
		return UserAccount{}, err
	}
	return account, nil
}

func (s *Service) IssueCredential(ctx context.Context, actorID, userID, requestID string) (CredentialIssue, error) {
	actorUUID, err := parseUUID(actorID)
	if err != nil {
		return CredentialIssue{}, ErrUnauthorized
	}
	userUUID, err := parseUUID(userID)
	if err != nil {
		return CredentialIssue{}, ErrInvalidInput
	}

	var issue CredentialIssue
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		user, err := q.GetUserByIDForUpdate(ctx, userUUID)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("find user for credential issue: %w", err)
		}
		if user.Status == "system" {
			return ErrForbidden
		}
		if user.Status == "locked" {
			return ErrUserNotReady
		}

		purpose, ttl := credentialPurposeReset, s.resetTTL
		if user.Status == "pending" {
			purpose, ttl = credentialPurposeInvite, s.inviteTTL
		}
		rawToken, tokenHash, err := newOpaqueToken()
		if err != nil {
			return err
		}
		expiresAt := time.Now().Add(ttl)
		if err := q.DeleteUnusedCredentialTokens(ctx, user.ID); err != nil {
			return fmt.Errorf("delete old credential tokens: %w", err)
		}
		if _, err := q.CreateCredentialToken(ctx, db.CreateCredentialTokenParams{
			UserID:    user.ID,
			Purpose:   purpose,
			TokenHash: tokenHash,
			ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
			CreatedBy: actorUUID,
		}); err != nil {
			return fmt.Errorf("create credential token: %w", err)
		}
		if err := s.auditData(ctx, q, actorUUID, "user.credential."+purpose, "user", user.ID,
			nil, map[string]any{"purpose": purpose, "expires_at": expiresAt}, requestID); err != nil {
			return fmt.Errorf("write credential audit: %w", err)
		}
		issue = CredentialIssue{Token: rawToken, Purpose: purpose, ExpiresAt: expiresAt}
		return nil
	})
	if err != nil {
		return CredentialIssue{}, err
	}
	return issue, nil
}

func (s *Service) Activate(ctx context.Context, rawToken, password, requestID string) error {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" || len(password) < minPasswordLength || len(password) > maxPasswordLength {
		return ErrInvalidInput
	}
	passwordHash, err := hashPassword(password, s.bcryptCost)
	if err != nil {
		return ErrInvalidInput
	}

	return database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		token, err := q.GetCredentialTokenByHashForUpdate(ctx, hashOpaqueToken(rawToken))
		if err != nil {
			if database.IsNotFound(err) {
				return ErrInvalidCredential
			}
			return fmt.Errorf("find credential token: %w", err)
		}
		if token.UsedAt.Valid || !token.ExpiresAt.Valid || !token.ExpiresAt.Time.After(time.Now()) {
			return ErrInvalidCredential
		}
		user, err := q.GetUserByIDForUpdate(ctx, token.UserID)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrInvalidCredential
			}
			return fmt.Errorf("find credential user: %w", err)
		}
		if user.Status == "system" {
			return ErrInvalidCredential
		}
		if user.Status == "locked" {
			return ErrUserNotReady
		}

		updated, err := q.UpdateUserPassword(ctx, db.UpdateUserPasswordParams{
			PasswordHash: pgtype.Text{String: passwordHash, Valid: true},
			ID:           user.ID,
		})
		if err != nil {
			return fmt.Errorf("set user password: %w", err)
		}
		action := "user.password.reset"
		if token.Purpose == credentialPurposeInvite && updated.Status == "pending" {
			updated, err = q.UpdateUserStatus(ctx, db.UpdateUserStatusParams{Status: "active", ID: user.ID})
			if err != nil {
				return fmt.Errorf("activate user: %w", err)
			}
			action = "user.activate"
		}
		if err := q.MarkCredentialTokenUsed(ctx, token.ID); err != nil {
			return fmt.Errorf("mark credential token used: %w", err)
		}
		if err := q.DeleteOtherUnusedCredentialTokens(ctx, db.DeleteOtherUnusedCredentialTokensParams{
			UserID: user.ID,
			KeepID: token.ID,
		}); err != nil {
			return fmt.Errorf("delete other credential tokens: %w", err)
		}
		if err := q.RevokeAllUserRefreshTokens(ctx, user.ID); err != nil {
			return fmt.Errorf("revoke user sessions after password change: %w", err)
		}
		if err := s.auditData(ctx, q, user.ID, action, "user", user.ID,
			nil, map[string]any{"status": updated.Status, "purpose": token.Purpose}, requestID); err != nil {
			return fmt.Errorf("write credential activation audit: %w", err)
		}
		return nil
	})
}

func (s *Service) ListRoles(ctx context.Context) ([]Role, error) {
	roles, err := s.queries.ListRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	result := make([]Role, 0, len(roles))
	for _, role := range roles {
		result = append(result, Role{Code: role.Code, Name: role.Name, Description: role.Description.String})
	}
	return result, nil
}

func (s *Service) validateRoleCodes(ctx context.Context, requested []string) ([]string, error) {
	if len(requested) == 0 {
		return nil, ErrInvalidInput
	}
	roles, err := s.queries.ListRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	known := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		known[role.Code] = struct{}{}
	}
	seen := make(map[string]struct{}, len(requested))
	result := make([]string, 0, len(requested))
	for _, code := range requested {
		code = strings.ToLower(strings.TrimSpace(code))
		if code == "" {
			return nil, ErrInvalidInput
		}
		if _, ok := known[code]; !ok {
			return nil, ErrInvalidInput
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		result = append(result, code)
	}
	sort.Strings(result)
	return result, nil
}

func normalizeUserInput(input CreateUserInput) (CreateUserInput, error) {
	input.Username = strings.TrimSpace(input.Username)
	input.Email = strings.TrimSpace(input.Email)
	input.FullName = strings.TrimSpace(input.FullName)
	if input.Username == "" || len(input.Username) > 100 || strings.IndexByte(input.Username, 0) >= 0 {
		return CreateUserInput{}, ErrInvalidInput
	}
	if input.FullName == "" || len(input.FullName) > 150 || strings.IndexByte(input.FullName, 0) >= 0 {
		return CreateUserInput{}, ErrInvalidInput
	}
	if len(input.Email) > 255 || strings.IndexByte(input.Email, 0) >= 0 {
		return CreateUserInput{}, ErrInvalidInput
	}
	if input.Email != "" {
		addr, err := mail.ParseAddress(input.Email)
		if err != nil || addr.Address != input.Email {
			return CreateUserInput{}, ErrInvalidInput
		}
	}
	return input, nil
}

func toUserAccount(user db.User, roles []string) UserAccount {
	return UserAccount{UserSummary: toUserSummary(user), Roles: roles}
}

func roleCodes(roles []db.Role) []string {
	result := make([]string, 0, len(roles))
	for _, role := range roles {
		result = append(result, role.Code)
	}
	sort.Strings(result)
	return result
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func userAuditData(user db.User, roles []string) map[string]any {
	return map[string]any{
		"username":  user.Username,
		"email":     user.Email.String,
		"full_name": user.FullName,
		"status":    user.Status,
		"roles":     roles,
	}
}

func (s *Service) auditData(ctx context.Context, q *db.Queries, actorID pgtype.UUID, action, entityType string, entityID pgtype.UUID, before, after any, requestID string) error {
	var beforeData, afterData []byte
	var err error
	if before != nil {
		beforeData, err = json.Marshal(before)
		if err != nil {
			return fmt.Errorf("encode audit before data: %w", err)
		}
	}
	if after != nil {
		afterData, err = json.Marshal(after)
		if err != nil {
			return fmt.Errorf("encode audit after data: %w", err)
		}
	}
	_, err = q.CreateAuditLog(ctx, db.CreateAuditLogParams{
		ActorUserID: actorID,
		Action:      action,
		EntityType:  entityType,
		EntityID:    entityID,
		RequestID:   optionalText(requestID),
		BeforeData:  string(beforeData),
		AfterData:   string(afterData),
	})
	return err
}
