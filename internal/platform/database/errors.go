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

func hasCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}
