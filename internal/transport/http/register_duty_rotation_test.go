package httptransport

import (
	"testing"

	"github.com/gin-gonic/gin"

	dutyrotationpkg "github.com/ghalitsar-coder/backend-ifter/internal/dutyrotation"
)

// TestRegisterDutyRotationRoutes is DB-free: it asserts the §6.3 contract
// routes (plus the package-spec uniform reports) are attached to the right
// capability groups and that the scaffold stays a no-op when the service is
// not wired.
func TestRegisterDutyRotationRoutes(t *testing.T) {
	log := discardLogger()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	v1 := engine.Group("/api/v1")
	groups := RouteGroups{
		AttendanceOps: v1.Group("/ops"),
		Authenticated: v1.Group(""),
	}

	RegisterDutyRotation(v1, groups, FeatureDeps{Log: log})
	if routes := engine.Routes(); len(routes) != 0 {
		t.Fatalf("scaffold registered %d routes without a service", len(routes))
	}

	deps := FeatureDeps{
		Log:          log,
		DutyRotation: dutyrotationpkg.NewService(nil, log),
	}
	ops := v1.Group("")
	authenticated := v1.Group("")
	RegisterDutyRotation(v1, RouteGroups{
		AttendanceOps: ops,
		Authenticated: authenticated,
	}, deps)

	registered := make(map[string]struct{})
	for _, route := range engine.Routes() {
		registered[route.Method+" "+route.Path] = struct{}{}
	}
	expected := []string{
		"GET /api/v1/duty-rotations",
		"POST /api/v1/duty-rotations",
		"PATCH /api/v1/duty-rotations/:id/cancel",
		"POST /api/v1/uniform-reports",
		"GET /api/v1/uniform-reports",
	}
	for _, route := range expected {
		if _, ok := registered[route]; !ok {
			t.Errorf("duty-rotation router is missing expected route: %s", route)
		}
	}
	for _, unexpected := range []string{
		"GET /api/v1/ops/duty-rotations",
		"DELETE /api/v1/duty-assignments/:id/substitutions/:subId",
		"POST /api/v1/duty-assignments/:id/substitutions",
		"GET /api/v1/duty-rotations/:id/cancel",
	} {
		if _, ok := registered[unexpected]; ok {
			t.Errorf("group prefix or wrong verb/path leaked into a route path: %s", unexpected)
		}
	}
}
