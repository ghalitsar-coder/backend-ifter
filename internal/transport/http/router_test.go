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

	attendancepkg "github.com/ghalitsar-coder/backend-ifter/internal/attendance"
	"github.com/ghalitsar-coder/backend-ifter/internal/auth"
	"github.com/ghalitsar-coder/backend-ifter/internal/config"
	disciplinepkg "github.com/ghalitsar-coder/backend-ifter/internal/discipline"
	importservice "github.com/ghalitsar-coder/backend-ifter/internal/imports"
	"github.com/ghalitsar-coder/backend-ifter/internal/masterdata"
	"github.com/ghalitsar-coder/backend-ifter/internal/reporting"
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

// TestRouterCapabilityEndpoints builds the full router (every service present, but
// with nil pools — only route registration runs, nothing is served) and asserts
// that the RBAC re-cut still exposes every existing endpoint across the new
// capability sub-groups. It is DB-free and always runs, and doubles as a guard
// against gin panicking on duplicate registrations.
func TestRouterCapabilityEndpoints(t *testing.T) {
	log := discardLogger()
	importService, err := importservice.NewService(nil, log, importservice.Config{Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("construct import service: %v", err)
	}

	router := New(
		log,
		fakeReadiness{},
		auth.NewService(nil, nil, auth.ServiceConfig{}, log),
		masterdata.NewService(nil, log),
		importService,
		attendancepkg.NewService(nil, log),
		disciplinepkg.NewService(nil, log),
		reporting.NewService(nil, log),
		config.AuthConfig{},
		nil,
	)

	registered := make(map[string]struct{})
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = struct{}{}
	}

	// Every existing endpoint, now distributed across the capability groups.
	// Presence (not exact equality) is the invariant this wave must preserve; the
	// seven register_*.go scaffolds may add routes later and extend this list.
	expected := []string{
		"GET /healthz", "GET /readyz", "GET /metrics",

		"POST /api/v1/auth/login", "POST /api/v1/auth/activate",
		"POST /api/v1/auth/refresh", "POST /api/v1/auth/logout", "GET /api/v1/auth/me",

		// Admin: user management, roles, audit.
		"GET /api/v1/users", "POST /api/v1/users", "GET /api/v1/users/:id",
		"PUT /api/v1/users/:id/roles", "PATCH /api/v1/users/:id/status",
		"POST /api/v1/users/:id/credential-reset", "GET /api/v1/roles",
		"GET /api/v1/audit-logs",

		// Imports group (admin + staff_tu).
		"GET /api/v1/imports", "POST /api/v1/imports/students",
		"POST /api/v1/imports/teachers", "GET /api/v1/imports/:id",
		"POST /api/v1/imports/:id/confirm", "GET /api/v1/import-templates/:entity",
		"GET /api/v1/exports/students", "GET /api/v1/exports/teachers",

		// Attendance ops (admin + teacher + duty_teacher).
		"GET /api/v1/attendance/roster", "POST /api/v1/attendance/bulk",
		"POST /api/v1/attendance/close", "POST /api/v1/attendance/records/:id/corrections",

		// Attendance read (+ principal).
		"GET /api/v1/attendance/history", "GET /api/v1/attendance/summary",
		"GET /api/v1/attendance/records/:id", "GET /api/v1/attendance/settings",
		"GET /api/v1/attendance/holidays",

		// Reports read (all staff).
		"GET /api/v1/exports/attendance", "GET /api/v1/reports/attendance",
		"GET /api/v1/exports/attendance-report", "GET /api/v1/dashboard",

		// Admin: attendance writes + duty master.
		"PUT /api/v1/attendance/settings", "POST /api/v1/attendance/holidays",
		"DELETE /api/v1/attendance/holidays/:date", "GET /api/v1/duty-assignments",
		"POST /api/v1/duty-assignments", "PUT /api/v1/duty-assignments/:id/end",

		// Discipline ops (admin + teacher + duty_teacher).
		"POST /api/v1/discipline/records", "POST /api/v1/discipline/records/:id/followups",

		// Discipline read (+ principal).
		"GET /api/v1/discipline/categories", "GET /api/v1/discipline/records",
		"GET /api/v1/discipline/records/:id", "GET /api/v1/discipline/students/:id",

		// Reports read for discipline.
		"GET /api/v1/exports/discipline", "GET /api/v1/reports/discipline",

		// Admin: discipline mgmt.
		"POST /api/v1/discipline/categories", "PUT /api/v1/discipline/categories/:id",
		"PATCH /api/v1/discipline/categories/:id/status", "PATCH /api/v1/discipline/records/:id/points",

		// Master data (admin + staff_tu).
		"GET /api/v1/academic-years", "POST /api/v1/academic-years",
		"GET /api/v1/academic-years/:id", "PUT /api/v1/academic-years/:id",
		"GET /api/v1/academic-years/:id/terms", "POST /api/v1/academic-years/:id/terms",
		"GET /api/v1/terms/:id", "PUT /api/v1/terms/:id",
		"GET /api/v1/classes", "POST /api/v1/classes", "GET /api/v1/classes/:id",
		"PUT /api/v1/classes/:id", "PATCH /api/v1/classes/:id/status",
		"GET /api/v1/classes/:id/students", "PUT /api/v1/classes/:id/homeroom",
		"GET /api/v1/teachers", "POST /api/v1/teachers", "GET /api/v1/teachers/:id",
		"PUT /api/v1/teachers/:id", "PATCH /api/v1/teachers/:id/status",
		"GET /api/v1/students", "POST /api/v1/students", "GET /api/v1/students/:id",
		"PUT /api/v1/students/:id", "PATCH /api/v1/students/:id/status",
		"PUT /api/v1/students/:id/class",
		"GET /api/v1/students/:id/notes", "POST /api/v1/students/:id/notes",
		"DELETE /api/v1/students/:id/notes/:noteId",
	}

	for _, route := range expected {
		if _, ok := registered[route]; !ok {
			t.Errorf("capability router is missing expected route: %s", route)
		}
	}
}
