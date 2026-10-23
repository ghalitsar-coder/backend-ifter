package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

// RateLimit is a per-client fixed-window limiter.
//
// shortcut: state lives in process memory, so each instance enforces its own
// budget. Move the counter to Redis before running more than one API replica.
func RateLimit(limit int, window time.Duration) gin.HandlerFunc {
	limiter := newWindowLimiter(limit, window)
	return func(c *gin.Context) {
		allowed, retryAfter := limiter.allow(c.ClientIP(), time.Now())
		if !allowed {
			c.Header("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
			response.Error(c, http.StatusTooManyRequests, "rate_limited", "too many requests")
			return
		}
		c.Next()
	}
}

type windowEntry struct {
	start time.Time
	count int
}

type windowLimiter struct {
	mu        sync.Mutex
	entries   map[string]windowEntry
	limit     int
	window    time.Duration
	lastSweep time.Time
}

func newWindowLimiter(limit int, window time.Duration) *windowLimiter {
	return &windowLimiter{
		entries: make(map[string]windowEntry),
		limit:   limit,
		window:  window,
	}
}

func (l *windowLimiter) allow(key string, now time.Time) (bool, time.Duration) {
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
		l.entries[key] = windowEntry{start: now, count: 1}
		return true, 0
	}
	if entry.count >= l.limit {
		return false, l.window - now.Sub(entry.start)
	}
	entry.count++
	l.entries[key] = entry
	return true, 0
}
