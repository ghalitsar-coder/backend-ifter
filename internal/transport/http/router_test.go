package httptransport

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ghalitsar-coder/backend-ifter/internal/config"
)

type fakeReadiness struct {
	err error
}

func (f fakeReadiness) Ping(context.Context) error {
	return f.err
}

func TestHealthz(t *testing.T) {
	router := New(discardLogger(), fakeReadiness{}, nil, nil, nil, nil, nil, nil, config.AuthConfig{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Request-ID", "test-request-id")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("X-Request-ID"); got != "test-request-id" {
		t.Fatalf("X-Request-ID = %q", got)
	}
}

func TestReadyz(t *testing.T) {
	tests := []struct {
		name       string
		readiness  Readiness
		wantStatus int
	}{
		{name: "ready", readiness: fakeReadiness{}, wantStatus: http.StatusOK},
		{name: "database down", readiness: fakeReadiness{err: errors.New("down")}, wantStatus: http.StatusServiceUnavailable},
		{name: "no readiness checker", readiness: nil, wantStatus: http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := New(discardLogger(), tt.readiness, nil, nil, nil, nil, nil, nil, config.AuthConfig{}, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestRequestIDIsGenerated(t *testing.T) {
	router := New(discardLogger(), fakeReadiness{}, nil, nil, nil, nil, nil, nil, config.AuthConfig{}, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if got := rec.Header().Get("X-Request-ID"); got == "" {
		t.Fatal("X-Request-ID is empty")
	}
}

func TestMetrics(t *testing.T) {
	router := New(discardLogger(), fakeReadiness{}, nil, nil, nil, nil, nil, nil, config.AuthConfig{}, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); !strings.Contains(body, "go_goroutines") {
		t.Fatalf("metrics body does not contain go_goroutines: %s", body)
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
