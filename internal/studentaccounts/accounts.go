package studentaccounts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

// Provision creates the login account of one student: an account with the
// "student" role plus an invite credential, both minted by the auth machinery
// this package delegates to (never a shared password), then links it through
// students.user_id and audits the write.
//
// Idempotency guard (package spec): a student that is already linked answers
// 409 instead of creating a second account.
func (s *Service) Provision(ctx context.Context, actor Actor, studentIDValue string, input ProvisionInput, requestID string) (ProvisionResult, error) {
	if err := requireProvisionActor(actor); err != nil {
		return ProvisionResult{}, err
	}
	if s.provisioner == nil {
		return ProvisionResult{}, ErrProvisionerUnavailable
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return ProvisionResult{}, ErrUnauthorized
	}
	studentID, err := parseUUID(studentIDValue)
	if err != nil {
		return ProvisionResult{}, ErrInvalidInput
	}
	username, err := cleanUsername(input.Username)
	if err != nil {
		return ProvisionResult{}, ErrInvalidInput
	}
	email, err := cleanEmail(input.Email)
	if err != nil {
		return ProvisionResult{}, ErrInvalidInput
	}

	// 1) read the student under a row lock and reject one that is already
	//    linked or no longer active.
	var student db.GetStudentForAccountLinkRow
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		row, err := db.New(tx).GetStudentForAccountLink(ctx, studentID)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("lock student for provisioning: %w", err)
		}
		if row.UserID.Valid {
			return ErrAlreadyLinked
		}
		if row.Status != studentStatus {
			return ErrStudentNotActive
		}
		student = row
		return nil
	})
	if err != nil {
		return ProvisionResult{}, err
	}
	if username == "" {
		// Package spec: the student number is the login name unless an admin
		// overrides it.
		username = student.StudentNumber
	}

	// 2) create user + roles + invite credential (auth-owned transaction).
	account, err := s.provisioner.CreateAccount(ctx, actor.ID, NewUserInput{
		Username: username,
		Email:    email,
		FullName: student.FullName,
		Roles:    []string{RoleStudent},
	}, requestID)
	if err != nil {
		return ProvisionResult{}, mapWriteError(err)
	}

	// 3) link + audit. The UPDATE is conditional on user_id IS NULL, so a link
	//    that appeared while the account was being created loses the race; any
	//    step-3 failure (lost race, audit error, database fault) revokes the
	//    fresh invite so the orphan account can never be activated.
	userID, err := parseUUID(account.User.ID)
	if err != nil {
		s.revokeInvite(ctx, account.User.ID)
		return ProvisionResult{}, ErrInvalidInput
	}
	var linked db.LinkStudentUserRow
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		if _, err := q.GetStudentForAccountLink(ctx, studentID); err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("relock student for linking: %w", err)
		}
		row, err := q.LinkStudentUser(ctx, db.LinkStudentUserParams{UserID: userID, ID: studentID})
		if err != nil {
			if database.IsNotFound(err) {
				return ErrAlreadyLinked
			}
			return mapWriteError(err)
		}
		linked = row
		after := accountViewOf(uuidString(linked.ID), linked.StudentNumber, uuidString(linked.UserID), username, linked.Status)
		return s.auditData(ctx, q, actorID, "student_account.create", studentID,
			accountViewOf(uuidString(student.ID), student.StudentNumber, "", "", student.Status), after, requestID)
	})
	if err != nil {
		// Any step-3 failure left the freshly created account pending with a
		// live one-time invite while the student is (still) unlinked. Revoke
		// the invite so no orphan credential can ever be activated. Best-effort:
		// revokeInvite logs, never masks the original error.
		s.revokeInvite(ctx, account.User.ID)
		return ProvisionResult{}, err
	}
	return ProvisionResult{User: account.User, Credential: account.Credential}, nil
}

// GetAccount returns the login link of one student for
// GET /students/:id/account: every field is null while no account exists.
func (s *Service) GetAccount(ctx context.Context, actor Actor, studentIDValue string) (Account, error) {
	if err := requireProvisionActor(actor); err != nil {
		return Account{}, err
	}
	studentID, err := parseUUID(studentIDValue)
	if err != nil {
		return Account{}, ErrInvalidInput
	}
	row, err := s.queries.GetStudentAccount(ctx, studentID)
	if err != nil {
		if database.IsNotFound(err) {
			return Account{}, ErrNotFound
		}
		return Account{}, fmt.Errorf("get student account: %w", err)
	}
	return Account{
		UserID:      uuidPtr(row.UserID),
		Username:    textPtr(row.Username),
		Status:      textPtr(row.UserStatus),
		LastLoginAt: timestampPtr(row.LastLoginAt),
	}, nil
}

// Deactivate withdraws a student's login: the user is set inactive (when it
// already has a password), its sessions and unused invite credentials are
// revoked, and the students.user_id link is cleared. The student row and all of
// its history are preserved, and a later re-provisioning starts from a clean
// link (docs/API_ADDENDUM.md §6.7).
func (s *Service) Deactivate(ctx context.Context, actor Actor, studentIDValue, requestID string) error {
	if err := requireProvisionActor(actor); err != nil {
		return err
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return ErrUnauthorized
	}
	studentID, err := parseUUID(studentIDValue)
	if err != nil {
		return ErrInvalidInput
	}
	return database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		student, err := q.GetStudentForAccountLink(ctx, studentID)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("lock student for deactivation: %w", err)
		}
		if !student.UserID.Valid {
			// Nothing to deactivate: the student never had (or no longer has)
			// a login account.
			return ErrNotFound
		}
		user, err := q.GetUserByIDForUpdate(ctx, student.UserID)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("lock student account user: %w", err)
		}
		if user.Status == "system" {
			return ErrForbidden
		}
		if err := q.RevokeAllUserRefreshTokens(ctx, user.ID); err != nil {
			return fmt.Errorf("revoke student account sessions: %w", err)
		}
		if err := q.DeleteUnusedCredentialTokens(ctx, user.ID); err != nil {
			return fmt.Errorf("revoke student account credentials: %w", err)
		}
		statusAfter := user.Status
		// users (000002) forbids a non-pending status without a password hash,
		// so a never-activated invite is withdrawn by unlinking + revoking the
		// token only.
		if user.PasswordHash.Valid && user.Status != "inactive" {
			updated, err := q.UpdateUserStatus(ctx, db.UpdateUserStatusParams{Status: "inactive", ID: user.ID})
			if err != nil {
				return fmt.Errorf("deactivate student account: %w", err)
			}
			statusAfter = updated.Status
		}
		unlinked, err := q.UnlinkStudentUser(ctx, studentID)
		if err != nil {
			return fmt.Errorf("unlink student account: %w", err)
		}
		before := accountViewOf(uuidString(student.ID), student.StudentNumber, uuidString(user.ID), user.Username, user.Status)
		after := accountViewOf(uuidString(unlinked.ID), unlinked.StudentNumber, "", user.Username, statusAfter)
		return s.auditData(ctx, q, actorID, "student_account.delete", studentID, before, after, requestID)
	})
}

// revokeInvite is the compensation for a lost provisioning race: the one-time
// token of the orphan account is dropped so it can never be activated.
// Best-effort by design; a failure is logged, not returned.
func (s *Service) revokeInvite(ctx context.Context, userID string) {
	id, err := parseUUID(userID)
	if err != nil {
		return
	}
	if err := database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		return db.New(tx).DeleteUnusedCredentialTokens(ctx, id)
	}); err != nil {
		s.log.Error("student_account_revoke_invite", "error", err)
	}
}
