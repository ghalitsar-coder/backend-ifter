package middleware

import (
	"testing"
	"time"
)

func TestLoginLimiter(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	limiter := &rateLimiter{
		entries: make(map[string]rateEntry),
		limit:   2,
		window:  time.Minute,
		now:     func() time.Time { return now },
	}

	if allowed, _ := limiter.allow("127.0.0.1", now); !allowed {
		t.Fatal("first attempt was rejected")
	}
	if allowed, _ := limiter.allow("127.0.0.1", now); !allowed {
		t.Fatal("second attempt was rejected")
	}
	if allowed, retryAfter := limiter.allow("127.0.0.1", now); allowed || retryAfter <= 0 {
		t.Fatalf("third attempt = allowed %v, retryAfter %v; want rejected with a retry delay", allowed, retryAfter)
	}
	if allowed, _ := limiter.allow("192.0.2.1", now); !allowed {
		t.Fatal("a different client was rejected")
	}

	now = now.Add(time.Minute)
	if allowed, _ := limiter.allow("127.0.0.1", now); !allowed {
		t.Fatal("attempt after the window was rejected")
	}
}
