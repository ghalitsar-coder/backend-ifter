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
// instance serves login traffic.
func LoginRateLimit(limit int, window time.Duration) gin.HandlerFunc {
	limiter := &loginLimiter{
		entries: make(map[string]loginEntry),
		limit:   limit,
		window:  window,
		now:     time.Now,
	}
	return func(c *gin.Context) {
		allowed, retryAfter := limiter.allow(c.ClientIP(), limiter.now())
		if !allowed {
			c.Header("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
			response.Error(c, http.StatusTooManyRequests, "rate_limited", "too many login attempts")
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

type loginEntry struct {
	start time.Time
	count int
}

type loginLimiter struct {
	mu        sync.Mutex
	entries   map[string]loginEntry
	limit     int
	window    time.Duration
	lastSweep time.Time
	now       func() time.Time
}

func (l *loginLimiter) allow(key string, now time.Time) (bool, time.Duration) {
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
		l.entries[key] = loginEntry{start: now, count: 1}
		return true, 0
	}
	if entry.count >= l.limit {
		return false, l.window - now.Sub(entry.start)
	}
	entry.count++
	l.entries[key] = entry
	return true, 0
}
