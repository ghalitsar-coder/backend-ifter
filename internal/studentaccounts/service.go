package studentaccounts

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
)

// ErrProvisionerUnavailable signals a wiring mistake (the router always
// supplies the auth-backed provisioner); it surfaces as a 500.
var ErrProvisionerUnavailable = errors.New("account provisioning is not configured")

// Provisioner is the slice of the auth package this feature needs to mint a
// login account and its activation credential: exactly the machinery behind
// POST /users and POST /auth/activate. The production implementation is the
// auth.Service adapter built in register_student_accounts.go; keeping it an
// interface here means the domain package does not depend on auth internals
// and stays unit-testable with a fake.
//
// Contract: CreateAccount returns this package's sentinel errors
// (ErrInvalidInput, ErrUnauthorized, ErrForbidden, ErrConflict, ErrNotFound)
// so the handler keeps a single error mapping; anything else is a 500.
type Provisioner interface {
	CreateAccount(ctx context.Context, actorID string, input NewUserInput, requestID string) (ProvisionedAccount, error)
}

// Service implements the student-account use cases
// (handler -> usecase -> repository).
type Service struct {
	pool        *pgxpool.Pool
	queries     *db.Queries
	log         *slog.Logger
	provisioner Provisioner
}

func NewService(pool *pgxpool.Pool, provisioner Provisioner, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		pool:        pool,
		queries:     db.New(pool),
		log:         log,
		provisioner: provisioner,
	}
}
