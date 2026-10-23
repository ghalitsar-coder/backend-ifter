package database

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsNotFound(t *testing.T) {
	if !IsNotFound(pgx.ErrNoRows) {
		t.Error("IsNotFound(pgx.ErrNoRows) = false, want true")
	}
	if !IsNotFound(fmt.Errorf("wrapped: %w", pgx.ErrNoRows)) {
		t.Error("IsNotFound(wrapped ErrNoRows) = false, want true")
	}
	if IsNotFound(errors.New("other")) {
		t.Error("IsNotFound(other) = true, want false")
	}
}

func TestPostgresErrorCodes(t *testing.T) {
	tests := []struct {
		name string
		code string
		got  func(error) bool
	}{
		{"unique", "23505", IsUniqueViolation},
		{"foreign key", "23503", IsForeignKeyViolation},
		{"check", "23514", IsCheckViolation},
		{"not null", "23502", IsNotNullViolation},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &pgconn.PgError{Code: tt.code}
			if !tt.got(err) {
				t.Errorf("check for %s = false, want true", tt.code)
			}
			if tt.got(&pgconn.PgError{Code: "00000"}) {
				t.Error("check matched an unrelated code")
			}
			if tt.got(errors.New("plain error")) {
				t.Error("check matched a non-Postgres error")
			}
		})
	}
}
