package database

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// IsNotFound reports whether err is pgx's no-rows sentinel.
func IsNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

func IsUniqueViolation(err error) bool {
	return hasCode(err, "23505")
}

func IsForeignKeyViolation(err error) bool {
	return hasCode(err, "23503")
}

func IsCheckViolation(err error) bool {
	return hasCode(err, "23514")
}

func IsNotNullViolation(err error) bool {
	return hasCode(err, "23502")
}

// ConstraintName returns the violated database constraint, when the error is a
// PostgreSQL error that carries one.
func ConstraintName(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.ConstraintName
	}
	return ""
}

func hasCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}
