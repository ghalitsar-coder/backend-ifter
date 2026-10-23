package middleware

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/platform/metrics"
)

// Metrics records latency and status using the registered route pattern.
func Metrics(m *metrics.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		m.IncInFlight()
		defer m.DecInFlight()

		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		m.ObserveRequest(c.Request.Method, route, c.Writer.Status(), time.Since(start).Seconds())
	}
}
