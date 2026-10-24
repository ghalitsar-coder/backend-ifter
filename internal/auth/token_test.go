package auth

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/ghalitsar-coder/backend-ifter/internal/config"
)

func newTestService(ttl time.Duration) *TokenService {
	return NewTokenService(config.AuthConfig{
		JWTSecret:      strings.Repeat("k", 32),
		JWTIssuer:      "test-issuer",
		JWTAudience:    "test-audience",
		AccessTokenTTL: ttl,
	})
}

func TestIssueAndParse(t *testing.T) {
	svc := newTestService(time.Minute)

	token, err := svc.Issue("user-123", []string{"admin", "staff"}, 7)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	claims, err := svc.Parse(token)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if claims.Subject != "user-123" {
		t.Errorf("Subject = %q, want user-123", claims.Subject)
	}
	if claims.SchoolID != 7 {
		t.Errorf("SchoolID = %d, want 7", claims.SchoolID)
	}
	if len(claims.Roles) != 2 || claims.Roles[0] != "admin" {
		t.Errorf("Roles = %v, want [admin staff]", claims.Roles)
	}
}

func TestParseRejectsTokenWithoutSchool(t *testing.T) {
	svc := newTestService(time.Minute)
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-123",
			Issuer:    "test-issuer",
			Audience:  jwt.ClaimStrings{"test-audience"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
	}).SignedString([]byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	if _, err := svc.Parse(token); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("Parse() error = %v, want ErrInvalidToken", err)
	}
}

func TestIssueRequiresSchool(t *testing.T) {
	if _, err := newTestService(time.Minute).Issue("user-123", nil, 0); err == nil {
		t.Fatal("Issue() succeeded without a school, want error")
	}
}

func TestParseRejectsExpiredToken(t *testing.T) {
	svc := newTestService(-time.Minute)

	token, err := svc.Issue("user-123", nil, 1)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	if _, err := svc.Parse(token); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("Parse() error = %v, want ErrInvalidToken", err)
	}
}

func TestParseRejectsWrongIssuer(t *testing.T) {
	issuer := newTestService(time.Minute)
	token, err := issuer.Issue("user-123", nil, 1)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	other := NewTokenService(config.AuthConfig{
		JWTSecret:      strings.Repeat("k", 32),
		JWTIssuer:      "other-issuer",
		JWTAudience:    "test-audience",
		AccessTokenTTL: time.Minute,
	})
	if _, err := other.Parse(token); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("Parse() error = %v, want ErrInvalidToken", err)
	}
}

func TestParseRejectsTamperedToken(t *testing.T) {
	svc := newTestService(time.Minute)
	token, err := svc.Issue("user-123", nil, 1)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	if _, err := svc.Parse(token + "tampered"); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("Parse() error = %v, want ErrInvalidToken", err)
	}
}

func TestIssueRequiresSubject(t *testing.T) {
	if _, err := newTestService(time.Minute).Issue("", nil, 1); err == nil {
		t.Fatal("Issue() succeeded without a subject, want error")
	}
}
