package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/platform/metrics"
)

func Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		metrics.ObserveRequest(c.Request.Method, route, strconv.Itoa(c.Writer.Status()), time.Since(start).Seconds())
	}
}
