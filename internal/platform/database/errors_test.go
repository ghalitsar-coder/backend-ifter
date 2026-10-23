package database

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestPostgresErrorMapping(t *testing.T) {
	tests := []struct {
		name string
		err  error
		got  func(error) bool
	}{
		{"not found", pgx.ErrNoRows, IsNotFound},
		{"unique", &pgconn.PgError{Code: "23505"}, IsUniqueViolation},
		{"foreign key", &pgconn.PgError{Code: "23503"}, IsForeignKeyViolation},
		{"exclusion", &pgconn.PgError{Code: "23P01"}, IsExclusionViolation},
		{"check", &pgconn.PgError{Code: "23514"}, IsCheckViolation},
		{"not null", &pgconn.PgError{Code: "23502"}, IsNotNullViolation},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.got(tt.err) {
				t.Fatalf("%s did not match %v", tt.name, tt.err)
			}
			if tt.got(errors.New("other")) {
				t.Fatalf("%s matched unrelated error", tt.name)
			}
		})
	}
}
