package auth

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/ghalitsar-coder/backend-ifter/internal/config"
)

type fakeRepository struct {
	userFn func(ctx context.Context, username string) (User, error)
}

func (f *fakeRepository) FindByUsername(ctx context.Context, username string) (User, error) {
	return f.userFn(ctx, username)
}

func testTokens() *TokenService {
	return NewTokenService(config.AuthConfig{
		JWTSecret:      strings.Repeat("k", 32),
		JWTIssuer:      "test-issuer",
		JWTAudience:    "test-audience",
		AccessTokenTTL: time.Hour,
	})
}

// storedUser hashes password so the service verifies against a real bcrypt
// hash instead of a made-up string.
func storedUser(t *testing.T, user User, password string) User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("generate hash: %v", err)
	}
	user.PasswordHash = string(hash)
	return user
}

func repoReturning(user User, err error) *fakeRepository {
	return &fakeRepository{
		userFn: func(context.Context, string) (User, error) {
			if err != nil {
				return User{}, err
			}
			return user, nil
		},
	}
}

func TestLoginIssuesTokenWithStoredRoles(t *testing.T) {
	tokens := testTokens()
	svc := NewLoginService(repoReturning(storedUser(t, User{
		ID:       1,
		SchoolID: 3,
		Username: "guru1",
		Nama:     "Ibu Sari",
		Roles:    []string{"guru"},
		IsActive: true,
	}, "guru123"), nil), tokens, time.Hour)

	result, err := svc.Login(context.Background(), "guru1", "guru123")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if result.UserID != 1 || result.Username != "guru1" || result.Nama != "Ibu Sari" {
		t.Errorf("profile = %+v, want user 1 guru1 Ibu Sari", result)
	}

	claims, err := tokens.Parse(result.Token)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if claims.Subject != strconv.FormatInt(1, 10) {
		t.Errorf("Subject = %q, want the users.id as text", claims.Subject)
	}
	if claims.SchoolID != 3 {
		t.Errorf("SchoolID = %d, want the stored school", claims.SchoolID)
	}
	if len(claims.Roles) != 1 || claims.Roles[0] != "guru" {
		t.Errorf("Roles = %v, want [guru]", claims.Roles)
	}

	expiresAt, err := time.Parse(time.RFC3339, result.ExpiresAt)
	if err != nil {
		t.Fatalf("expires_at %q is not RFC3339: %v", result.ExpiresAt, err)
	}
	if delta := expiresAt.Sub(time.Now().UTC()); delta < 55*time.Minute || delta > time.Hour {
		t.Errorf("expires_at is %v ahead, want about 1h", delta)
	}
}

func TestLoginRejectsBadCredentials(t *testing.T) {
	found := storedUser(t, User{ID: 1, SchoolID: 1, Username: "guru1", Roles: []string{"guru"}, IsActive: true}, "guru123")

	tests := []struct {
		name     string
		repo     *fakeRepository
		password string
	}{
		{"wrong password", repoReturning(found, nil), "salah"},
		{"unknown username", repoReturning(User{}, pgx.ErrNoRows), "guru123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewLoginService(tt.repo, testTokens(), time.Hour)
			_, err := svc.Login(context.Background(), "guru1", tt.password)
			if !errors.Is(err, ErrInvalidCredentials) {
				t.Fatalf("error = %v, want ErrInvalidCredentials", err)
			}
		})
	}
}

func TestLoginRejectsDisabledAccount(t *testing.T) {
	user := storedUser(t, User{ID: 1, SchoolID: 1, Username: "guru1", Roles: []string{"guru"}, IsActive: false}, "guru123")
	svc := NewLoginService(repoReturning(user, nil), testTokens(), time.Hour)

	if _, err := svc.Login(context.Background(), "guru1", "guru123"); !errors.Is(err, ErrAccountDisabled) {
		t.Fatalf("error = %v, want ErrAccountDisabled", err)
	}
}

func TestLoginRejectsUserWithoutRoles(t *testing.T) {
	user := storedUser(t, User{ID: 1, SchoolID: 1, Username: "guru1", IsActive: true}, "guru123")
	svc := NewLoginService(repoReturning(user, nil), testTokens(), time.Hour)

	if _, err := svc.Login(context.Background(), "guru1", "guru123"); !errors.Is(err, ErrNoRole) {
		t.Fatalf("error = %v, want ErrNoRole", err)
	}
}

func TestLoginChecksPasswordBeforeAccountState(t *testing.T) {
	// A disabled account must not be revealed to a caller with the wrong password.
	user := storedUser(t, User{ID: 1, SchoolID: 1, Username: "guru1", Roles: []string{"guru"}, IsActive: false}, "guru123")
	svc := NewLoginService(repoReturning(user, nil), testTokens(), time.Hour)

	if _, err := svc.Login(context.Background(), "guru1", "salah"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("error = %v, want ErrInvalidCredentials", err)
	}
}

func TestLoginPropagatesRepositoryError(t *testing.T) {
	svc := NewLoginService(repoReturning(User{}, errors.New("connection reset")), testTokens(), time.Hour)

	if _, err := svc.Login(context.Background(), "guru1", "guru123"); err == nil ||
		errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("error = %v, want the wrapped repository error", err)
	}
}
