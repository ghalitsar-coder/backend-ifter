package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

// RequestID propagates a caller-supplied correlation ID or generates one, then
// echoes it back so logs can be joined with client reports.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.GetHeader(response.RequestIDHeader))
		if id == "" || len(id) > 128 {
			id = uuid.NewString()
		}
		c.Header(response.RequestIDHeader, id)
		response.SetRequestID(c, id)
		c.Next()
	}
}
