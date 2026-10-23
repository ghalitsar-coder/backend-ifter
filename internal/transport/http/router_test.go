package httptransport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/config"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/metrics"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeReadiness struct {
	err error
}

func (f fakeReadiness) Ping(context.Context) error {
	return f.err
}

func testDeps() Deps {
	return Deps{
		Config: &config.Config{
			App: config.AppConfig{Name: "test", Env: "test"},
			HTTP: config.HTTPConfig{
				MaxBodyBytes:      1 << 20,
				RateLimitRequests: 100,
				RateLimitWindow:   time.Minute,
			},
		},
		Logger:    testLogger(),
		Metrics:   metrics.New(),
		Readiness: fakeReadiness{},
		Version:   "test",
	}
}

func TestRouterStatusEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := New(testDeps())

	tests := []struct {
		method string
		path   string
		want   int
	}{
		{http.MethodGet, "/healthz", http.StatusOK},
		{http.MethodGet, "/readyz", http.StatusOK},
		{http.MethodGet, "/", http.StatusOK},
		{http.MethodGet, "/v1/version", http.StatusOK},
		{http.MethodGet, "/metrics", http.StatusOK},
		{http.MethodGet, "/missing", http.StatusNotFound},
		{http.MethodPost, "/healthz", http.StatusMethodNotAllowed},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestRouterMetricsExposed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := New(testDeps())

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(rec.Body.String(), "go_goroutines") {
		t.Error("metrics body does not contain go_goroutines")
	}
}

func TestRouterReadinessFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	deps := testDeps()
	deps.Readiness = fakeReadiness{err: errors.New("database down")}

	rec := httptest.NewRecorder()
	New(deps).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestRouterRequestIDAndErrorEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := New(testDeps())

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	req.Header.Set("X-Request-ID", "req-123")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Request-ID"); got != "req-123" {
		t.Errorf("X-Request-ID = %q, want req-123", got)
	}

	var body struct {
		Error struct {
			Code      string `json:"code"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error.Code != "not_found" {
		t.Errorf("error.code = %q, want not_found", body.Error.Code)
	}
	if body.Error.RequestID != "req-123" {
		t.Errorf("error.request_id = %q, want req-123", body.Error.RequestID)
	}
}
