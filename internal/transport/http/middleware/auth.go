package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/auth"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

const userContextKey = "auth_user"

type Authenticator interface {
	Authenticate(ctx context.Context, accessToken string) (auth.User, error)
}

func Authenticate(authenticator Authenticator, log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		fields := strings.Fields(c.GetHeader("Authorization"))
		if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
			unauthorized(c)
			return
		}

		user, err := authenticator.Authenticate(c.Request.Context(), fields[1])
		if err != nil {
			if !errors.Is(err, auth.ErrUnauthorized) {
				log.Error("authentication_failed", "request_id", response.RequestID(c), "error", err)
			}
			unauthorized(c)
			return
		}
		c.Set(userContextKey, user)
		c.Next()
	}
}

func RequireRoles(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}

	return func(c *gin.Context) {
		user, ok := CurrentUser(c)
		if !ok {
			unauthorized(c)
			return
		}
		for _, role := range user.Roles {
			if _, ok := allowed[role]; ok {
				c.Next()
				return
			}
		}
		response.Error(c, http.StatusForbidden, "forbidden", "forbidden")
	}
}

func CurrentUser(c *gin.Context) (auth.User, bool) {
	value, ok := c.Get(userContextKey)
	if !ok {
		return auth.User{}, false
	}
	user, ok := value.(auth.User)
	return user, ok
}

func unauthorized(c *gin.Context) {
	c.Header("WWW-Authenticate", "Bearer")
	response.Error(c, http.StatusUnauthorized, "unauthorized", "authentication required")
}
