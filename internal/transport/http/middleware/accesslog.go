package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

// AccessLog emits one structured line per request. Health and metrics probes
// are logged at debug level to keep production logs useful.
func AccessLog(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		attrs := []any{
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", response.RequestID(c),
			"client_ip", c.ClientIP(),
		}
		switch c.Request.URL.Path {
		case "/healthz", "/readyz", "/metrics":
			log.Debug("http_request", attrs...)
		default:
			log.Info("http_request", attrs...)
		}
	}
}
