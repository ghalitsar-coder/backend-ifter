package auth

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
)

// Repository is the persistence contract LoginService depends on. Tests use a
// fake; production uses PostgresRepository.
type Repository interface {
	FindByUsername(ctx context.Context, username string) (User, error)
}

type PostgresRepository struct {
	queries *db.Queries
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{queries: db.New(pool)}
}

func (r *PostgresRepository) FindByUsername(ctx context.Context, username string) (User, error) {
	row, err := r.queries.FindUserForLogin(ctx, username)
	if err != nil {
		return User{}, err
	}
	return User{
		ID:           row.ID,
		SchoolID:     row.SchoolID,
		Username:     row.Username,
		PasswordHash: row.PasswordHash,
		IsActive:     row.IsActive,
		Nama:         row.Nama,
		Roles:        row.Roles,
	}, nil
}

func (r *PostgresRepository) RegisterSchoolAdmin(ctx context.Context, in RegisterInput, username, passwordHash string) (RegisterResult, error) {
	row, err := r.queries.RegisterSchoolAdmin(ctx, db.RegisterSchoolAdminParams{
		Npsn:         in.NPSN,
		SchoolName:   in.SchoolName,
		Jenjang:      in.Jenjang,
		Username:     username,
		PasswordHash: passwordHash,
		Nama:         in.AdminName,
		NoHp:         &in.Phone,
	})
	if err != nil {
		return RegisterResult{}, err
	}
	return RegisterResult{UserID: row.ID, SchoolID: row.SchoolID, Username: row.Username}, nil
}
