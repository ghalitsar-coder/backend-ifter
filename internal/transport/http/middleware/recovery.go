package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

// Recovery converts a panic into a logged 500 so one bad request cannot take
// down the process.
func Recovery(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}
			log.Error("panic_recovered",
				"request_id", response.RequestID(c),
				"method", c.Request.Method,
				"path", c.Request.URL.Path,
				"panic", recovered,
				"stack", string(debug.Stack()),
			)
			response.Error(c, http.StatusInternalServerError, "internal_error", "internal server error")
		}()
		c.Next()
	}
}
