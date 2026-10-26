package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

// shortcut: process-local rate limit, move to Redis when more than one API
// instance serves the same clients.
func LoginRateLimit(limit int, window time.Duration) gin.HandlerFunc {
	return ipRateLimit(limit, window, "too many login attempts")
}

// IPRateLimit caps requests per client IP for unauthenticated ingest surfaces.
func IPRateLimit(limit int, window time.Duration) gin.HandlerFunc {
	return ipRateLimit(limit, window, "too many requests")
}

func ipRateLimit(limit int, window time.Duration, message string) gin.HandlerFunc {
	limiter := &rateLimiter{
		entries: make(map[string]rateEntry),
		limit:   limit,
		window:  window,
		now:     time.Now,
	}
	return func(c *gin.Context) {
		allowed, retryAfter := limiter.allow(c.ClientIP(), limiter.now())
		if !allowed {
			c.Header("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
			response.Error(c, http.StatusTooManyRequests, "rate_limited", message)
			return
		}
		c.Next()
	}
}

func BodyLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		}
		c.Next()
	}
}

type rateEntry struct {
	start time.Time
	count int
}

type rateLimiter struct {
	mu        sync.Mutex
	entries   map[string]rateEntry
	limit     int
	window    time.Duration
	lastSweep time.Time
	now       func() time.Time
}

func (l *rateLimiter) allow(key string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.lastSweep.IsZero() || now.Sub(l.lastSweep) >= l.window {
		for entryKey, entry := range l.entries {
			if now.Sub(entry.start) >= 2*l.window {
				delete(l.entries, entryKey)
			}
		}
		l.lastSweep = now
	}

	entry, ok := l.entries[key]
	if !ok || now.Sub(entry.start) >= l.window {
		l.entries[key] = rateEntry{start: now, count: 1}
		return true, 0
	}
	if entry.count >= l.limit {
		return false, l.window - now.Sub(entry.start)
	}
	entry.count++
	l.entries[key] = entry
	return true, 0
}
