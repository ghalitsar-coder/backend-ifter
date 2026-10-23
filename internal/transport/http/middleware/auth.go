package middleware

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/auth"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

const claimsContextKey = "auth_claims"

// TokenParser is the narrow contract the HTTP layer needs from the auth
// service, which keeps middleware testable without a token store.
type TokenParser interface {
	Parse(raw string) (*auth.Claims, error)
}

func Authenticate(parser TokenParser, log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, ok := bearerToken(c.GetHeader("Authorization"))
		if !ok {
			unauthorized(c)
			return
		}

		claims, err := parser.Parse(raw)
		if err != nil {
			log.Warn("invalid_access_token",
				"request_id", response.RequestID(c),
				"path", c.Request.URL.Path,
				"error", err,
			)
			unauthorized(c)
			return
		}
		c.Set(claimsContextKey, claims)
		c.Next()
	}
}

func RequireRoles(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}

	return func(c *gin.Context) {
		claims, ok := CurrentClaims(c)
		if !ok {
			unauthorized(c)
			return
		}
		for _, role := range claims.Roles {
			if _, ok := allowed[role]; ok {
				c.Next()
				return
			}
		}
		response.Error(c, http.StatusForbidden, "forbidden", "insufficient permissions")
	}
}

func CurrentClaims(c *gin.Context) (*auth.Claims, bool) {
	value, ok := c.Get(claimsContextKey)
	if !ok {
		return nil, false
	}
	claims, ok := value.(*auth.Claims)
	return claims, ok
}

func bearerToken(header string) (string, bool) {
	fields := strings.Fields(header)
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
		return "", false
	}
	return fields[1], true
}

func unauthorized(c *gin.Context) {
	c.Header("WWW-Authenticate", "Bearer")
	response.Error(c, http.StatusUnauthorized, "unauthorized", "authentication required")
}
