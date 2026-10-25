package httptransport

import (
	"testing"

	"github.com/gin-gonic/gin"

	dispensationpkg "github.com/ghalitsar-coder/backend-ifter/internal/dispensations"
)

// TestRegisterDispensationsRoutes is DB-free: it asserts the §6.2 contract
// routes are attached to the right capability groups and that the scaffold
// stays a no-op when the service is not wired.
func TestRegisterDispensationsRoutes(t *testing.T) {
	log := discardLogger()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	v1 := engine.Group("/api/v1")
	groups := RouteGroups{
		DispensationsRead: v1.Group("/read"),
		AttendanceOps:     v1.Group("/ops"),
	}

	RegisterDispensations(v1, groups, FeatureDeps{Log: log})
	if routes := engine.Routes(); len(routes) != 0 {
		t.Fatalf("scaffold registered %d routes without a service", len(routes))
	}

	deps := FeatureDeps{
		Log: log,
		Dispensations: dispensationpkg.NewService(nil, log, dispensationpkg.Config{
			Dir: t.TempDir(),
		}),
	}
	read := v1.Group("")
	ops := v1.Group("")
	authenticated := v1.Group("")
	my := v1.Group("")
	RegisterDispensations(v1, RouteGroups{
		DispensationsRead: read,
		AttendanceOps:     ops,
		Authenticated:     authenticated,
		My:                my,
	}, deps)

	registered := make(map[string]struct{})
	for _, route := range engine.Routes() {
		registered[route.Method+" "+route.Path] = struct{}{}
	}
	expected := []string{
		"GET /api/v1/dispensations",
		"GET /api/v1/dispensations/:id",
		"GET /api/v1/dispensations/:id/document",
		"POST /api/v1/dispensations/:id/verify",
		"POST /api/v1/dispensations",
		"GET /api/v1/my/dispensations",
		"POST /api/v1/my/dispensations",
	}
	for _, route := range expected {
		if _, ok := registered[route]; !ok {
			t.Errorf("dispensation router is missing expected route: %s", route)
		}
	}
	for _, unexpected := range []string{
		"GET /api/v1/read/dispensations",
		"POST /api/v1/ops/dispensations",
		"PATCH /api/v1/dispensations/:id/verify",
	} {
		if _, ok := registered[unexpected]; ok {
			t.Errorf("group prefix or wrong verb leaked into a route path: %s", unexpected)
		}
	}
}
