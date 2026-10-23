package middleware

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/auth"
	"github.com/ghalitsar-coder/backend-ifter/internal/config"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testTokens() *auth.TokenService {
	return auth.NewTokenService(config.AuthConfig{
		JWTSecret:      strings.Repeat("k", 32),
		JWTIssuer:      "test-issuer",
		JWTAudience:    "test-audience",
		AccessTokenTTL: time.Minute,
	})
}

func TestAuthenticateAndRequireRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokens := testTokens()

	router := gin.New()
	router.Use(RequestID(), Authenticate(tokens, testLogger()))
	router.GET("/me", func(c *gin.Context) {
		claims, ok := CurrentClaims(c)
		if !ok {
			response.Error(c, http.StatusInternalServerError, "missing_claims", "missing claims")
			return
		}
		response.JSON(c, http.StatusOK, gin.H{"sub": claims.Subject})
	})
	router.GET("/admin", RequireRoles("admin"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	adminToken, err := tokens.Issue("admin-1", []string{"admin"})
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	viewerToken, err := tokens.Issue("viewer-1", []string{"viewer"})
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	tests := []struct {
		name   string
		path   string
		header string
		want   int
	}{
		{"valid token", "/me", "Bearer " + adminToken, http.StatusOK},
		{"missing header", "/me", "", http.StatusUnauthorized},
		{"wrong scheme", "/me", "Token " + adminToken, http.StatusUnauthorized},
		{"garbage token", "/me", "Bearer not-a-token", http.StatusUnauthorized},
		{"role allowed", "/admin", "Bearer " + adminToken, http.StatusOK},
		{"role denied", "/admin", "Bearer " + viewerToken, http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestRequireRolesWithoutClaims(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/orphan", RequireRoles("admin"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/orphan", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
