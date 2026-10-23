package auth

import (
	"strings"
	"testing"
	"time"
)

func TestTokenManagerIssueAndParse(t *testing.T) {
	manager := NewTokenManager("test-secret-at-least-32-characters-long", "test-issuer", 15*time.Minute)

	token, expiresAt, err := manager.Issue("4f0c1f8e-2d5f-4b7f-9a1f-4c3a9f8b2e10")
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if token == "" || time.Until(expiresAt) <= 0 {
		t.Fatal("Issue() returned an empty token or a past expiry")
	}

	subject, err := manager.Parse(token)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if subject != "4f0c1f8e-2d5f-4b7f-9a1f-4c3a9f8b2e10" {
		t.Fatalf("Parse() subject = %q", subject)
	}
}

func TestTokenManagerRejectsInvalidTokens(t *testing.T) {
	manager := NewTokenManager("test-secret-at-least-32-characters-long", "test-issuer", 15*time.Minute)
	valid, _, err := manager.Issue("4f0c1f8e-2d5f-4b7f-9a1f-4c3a9f8b2e10")
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	tests := []struct {
		name    string
		manager *TokenManager
		token   string
	}{
		{name: "tampered", manager: manager, token: valid + "x"},
		{name: "wrong issuer", manager: NewTokenManager("test-secret-at-least-32-characters-long", "other-issuer", 15*time.Minute), token: valid},
		{name: "expired", manager: NewTokenManager("test-secret-at-least-32-characters-long", "test-issuer", -time.Minute), token: mustIssue(t, NewTokenManager("test-secret-at-least-32-characters-long", "test-issuer", -time.Minute))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tt.manager.Parse(tt.token); err == nil {
				t.Fatal("Parse() error = nil, want error")
			}
		})
	}
}

func TestRefreshTokenGeneration(t *testing.T) {
	rawA, hashA, err := newOpaqueToken()
	if err != nil {
		t.Fatalf("newOpaqueToken() error = %v", err)
	}
	rawB, hashB, err := newOpaqueToken()
	if err != nil {
		t.Fatalf("newOpaqueToken() error = %v", err)
	}
	if rawA == rawB || hashA == hashB {
		t.Fatal("refresh tokens are not unique")
	}
	if len(hashA) != 64 || strings.Trim(hashA, "0123456789abcdef") != "" {
		t.Fatalf("hash = %q, want 64 hex characters", hashA)
	}
	if hashOpaqueToken(rawA) != hashA {
		t.Fatal("hashOpaqueToken() does not match stored hash")
	}
}

func TestPasswordHashing(t *testing.T) {
	hash, err := hashPassword("correct horse battery staple", 4)
	if err != nil {
		t.Fatalf("hashPassword() error = %v", err)
	}
	if hash == "correct horse battery staple" {
		t.Fatal("password was stored in plain text")
	}
	if !checkPassword(hash, "correct horse battery staple") {
		t.Fatal("checkPassword() rejected the correct password")
	}
	if checkPassword(hash, "wrong password") {
		t.Fatal("checkPassword() accepted a wrong password")
	}
}

func mustIssue(t *testing.T, manager *TokenManager) string {
	t.Helper()
	token, _, err := manager.Issue("4f0c1f8e-2d5f-4b7f-9a1f-4c3a9f8b2e10")
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	return token
}
