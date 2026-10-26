package middleware

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

// SecurityHeaders adds the response headers an API needs by default. HSTS is
// only set when TLS terminated on this server; deployments behind a TLS proxy
// should add it at the proxy instead.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.Writer.Header()
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("X-Frame-Options", "DENY")
		header.Set("Referrer-Policy", "no-referrer")
		header.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		if c.Request.TLS != nil {
			header.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		c.Next()
	}
}

// BearerTokenAuth guards endpoints with one static token, such as /metrics.
// Both sides are hashed so the comparison does not leak the token length.
func BearerTokenAuth(expected string) gin.HandlerFunc {
	expectedSum := sha256.Sum256([]byte(expected))
	return func(c *gin.Context) {
		const prefix = "Bearer "
		header := c.GetHeader("Authorization")
		presented := ""
		if strings.HasPrefix(header, prefix) {
			presented = header[len(prefix):]
		}
		presentedSum := sha256.Sum256([]byte(presented))
		if subtle.ConstantTimeCompare(presentedSum[:], expectedSum[:]) != 1 {
			c.Header("WWW-Authenticate", "Bearer")
			response.Error(c, http.StatusUnauthorized, "unauthorized", "authentication required")
			c.Abort()
			return
		}
		c.Next()
	}
}
