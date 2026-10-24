package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"

	"github.com/ghalitsar-coder/backend-ifter/internal/config"
)

type fakeRegisterRepository struct {
	created RegisterResult
	err     error

	gotInput    RegisterInput
	gotUsername string
	gotHash     string
}

func (f *fakeRegisterRepository) RegisterSchoolAdmin(_ context.Context, in RegisterInput, username, passwordHash string) (RegisterResult, error) {
	f.gotInput = in
	f.gotUsername = username
	f.gotHash = passwordHash
	return f.created, f.err
}

func testRegisterTokens() *TokenService {
	return NewTokenService(config.AuthConfig{
		JWTSecret:      "0123456789abcdef0123456789abcdef",
		JWTIssuer:      "test-issuer",
		JWTAudience:    "test-audience",
		AccessTokenTTL: time.Hour,
	})
}

func validRegistration() RegisterInput {
	return RegisterInput{
		SchoolName: "  SMP Negeri 1 Sleman ",
		NPSN:       " 12345678 ",
		Jenjang:    "smp",
		AdminName:  " Budi Santoso ",
		Email:      " Admin@Sekolah.Sch.ID ",
		Phone:      "0812-3456-7890",
		Password:   "rahasia123",
	}
}

func TestRegisterCreatesAdminSession(t *testing.T) {
	repo := &fakeRegisterRepository{created: RegisterResult{UserID: 7, SchoolID: 2, Username: "admin@sekolah.sch.id"}}
	tokens := testRegisterTokens()
	svc := NewRegisterService(repo, tokens, time.Hour)

	result, err := svc.Register(context.Background(), validRegistration())
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if repo.gotInput.NPSN != "12345678" || repo.gotInput.Jenjang != "SMP" ||
		repo.gotInput.AdminName != "Budi Santoso" || repo.gotInput.Phone != "081234567890" {
		t.Errorf("normalized input = %+v", repo.gotInput)
	}
	if repo.gotUsername != "admin@sekolah.sch.id" {
		t.Errorf("username = %q, want the lowercased email", repo.gotUsername)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(repo.gotHash), []byte("rahasia123")); err != nil {
		t.Errorf("stored hash does not match the password: %v", err)
	}
	if result.Token == "" || result.UserID != 7 || result.Username != "admin@sekolah.sch.id" ||
		result.Nama != "Budi Santoso" || len(result.Roles) != 1 || result.Roles[0] != "admin" {
		t.Fatalf("result = %+v", result)
	}

	claims, err := tokens.Parse(result.Token)
	if err != nil {
		t.Fatalf("token does not parse: %v", err)
	}
	if claims.Subject != "7" || claims.SchoolID != 2 || len(claims.Roles) != 1 || claims.Roles[0] != "admin" {
		t.Errorf("claims = %+v, want subject 7, school 2, admin", claims)
	}
}

func TestRegisterValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*RegisterInput)
	}{
		{"empty school", func(in *RegisterInput) { in.SchoolName = " " }},
		{"short npsn", func(in *RegisterInput) { in.NPSN = "1234567" }},
		{"non-numeric npsn", func(in *RegisterInput) { in.NPSN = "1234abcd" }},
		{"bad email", func(in *RegisterInput) { in.Email = "bukan-email" }},
		{"short phone", func(in *RegisterInput) { in.Phone = "0812" }},
		{"foreign phone", func(in *RegisterInput) { in.Phone = "1234567890" }},
		{"short password", func(in *RegisterInput) { in.Password = "pendek" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRegisterRepository{}
			svc := NewRegisterService(repo, testRegisterTokens(), time.Hour)

			in := validRegistration()
			tt.mutate(&in)

			if _, err := svc.Register(context.Background(), in); !errors.Is(err, ErrInvalidRegistration) {
				t.Fatalf("Register() error = %v, want ErrInvalidRegistration", err)
			}
			if repo.gotUsername != "" {
				t.Error("repository was called for invalid input")
			}
		})
	}
}

func TestRegisterMapsDuplicateConstraints(t *testing.T) {
	tests := []struct {
		constraint string
		want       error
	}{
		{"uq_users_username", ErrEmailTaken},
		{"uq_schools_npsn", ErrNPSNTaken},
	}

	for _, tt := range tests {
		t.Run(tt.constraint, func(t *testing.T) {
			repo := &fakeRegisterRepository{err: &pgconn.PgError{Code: "23505", ConstraintName: tt.constraint}}
			svc := NewRegisterService(repo, testRegisterTokens(), time.Hour)

			if _, err := svc.Register(context.Background(), validRegistration()); !errors.Is(err, tt.want) {
				t.Fatalf("Register() error = %v, want %v", err, tt.want)
			}
		})
	}
}
