package httptransport

import (
	"errors"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/auth"
	studentaccountspkg "github.com/ghalitsar-coder/backend-ifter/internal/studentaccounts"
)

// TestRegisterStudentAccountsRoutes is DB-free: it asserts the §6.7 contract
// paths (plus the package spec's /my/* aliases) are attached verbatim on the
// admin-only provisioning surface, and that the register stays a no-op when no
// student-accounts service is wired in FeatureDeps.
func TestRegisterStudentAccountsRoutes(t *testing.T) {
	log := discardLogger()

	engine := gin.New()
	v1, groups := newStudentAccountTestGroups(engine)
	RegisterStudentAccounts(v1, groups, FeatureDeps{Log: log})
	if routes := engine.Routes(); len(routes) != 0 {
		t.Fatalf("register produced %d routes without a service", len(routes))
	}

	service := studentaccountspkg.NewService(nil, nil, log)
	engine = gin.New()
	v1, groups = newStudentAccountTestGroups(engine)
	RegisterStudentAccounts(v1, groups, FeatureDeps{Log: log, StudentAccounts: service})

	expected := []string{
		"POST /api/v1/students/:id/account",
		"GET /api/v1/students/:id/account",
		"DELETE /api/v1/students/:id/account",
		"GET /api/v1/my/profile",
		"GET /api/v1/my/attendance",
		"GET /api/v1/my/attendance-history",
		"GET /api/v1/my/attendance-summary",
		"GET /api/v1/my/discipline",
		"GET /api/v1/my/discipline-summary",
	}
	registered := map[string]struct{}{}
	for _, route := range engine.Routes() {
		registered[route.Method+" "+route.Path] = struct{}{}
	}
	for _, route := range expected {
		if _, ok := registered[route]; !ok {
			t.Errorf("missing route %s", route)
		}
	}
	if len(registered) != len(expected) {
		t.Errorf("registered %d routes, want %d: %v", len(registered), len(expected), registered)
	}
	for _, unexpected := range []string{
		"PUT /api/v1/students/:id/account",
		"PATCH /api/v1/students/:id/account",
		"POST /api/v1/my/attendance",
		"DELETE /api/v1/my/discipline",
		"GET /api/v1/my/dispensations",
		"POST /api/v1/my/dispensations",
		"GET /api/v1/students/:id",
	} {
		if _, ok := registered[unexpected]; ok {
			t.Errorf("unexpected route registered: %s", unexpected)
		}
	}
}

// TestStudentAccountsProvisionerAdapterFoldsAuthErrors covers the register
// file's adapter contract: the auth vocabulary must reach the handler as this
// package's sentinels.
func TestStudentAccountsProvisionerAdapterFoldsAuthErrors(t *testing.T) {
	cases := []struct {
		in   error
		want error
	}{
		{auth.ErrInvalidInput, studentaccountspkg.ErrInvalidInput},
		{auth.ErrUnauthorized, studentaccountspkg.ErrUnauthorized},
		{auth.ErrForbidden, studentaccountspkg.ErrForbidden},
		{auth.ErrNotFound, studentaccountspkg.ErrNotFound},
		{auth.ErrConflict, studentaccountspkg.ErrConflict},
	}
	for _, tt := range cases {
		if got := mapAuthAccountError(tt.in); !errors.Is(got, tt.want) {
			t.Errorf("mapAuthAccountError(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
	if got := mapAuthAccountError(studentaccountspkg.ErrAlreadyLinked); !errors.Is(got, studentaccountspkg.ErrAlreadyLinked) {
		t.Errorf("unknown errors must pass through unchanged, got %v", got)
	}
}

func newStudentAccountTestGroups(engine *gin.Engine) (*gin.RouterGroup, RouteGroups) {
	v1 := engine.Group("/api/v1")
	// The shape test does not exercise the guards, so plain sub-groups stand in
	// for the roleGroup derivatives router.New() builds.
	return v1, RouteGroups{
		V1:         v1,
		Admin:      v1.Group(""),
		MasterData: v1.Group(""),
		My:         v1.Group(""),
	}
}
