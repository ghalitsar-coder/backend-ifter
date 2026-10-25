package httptransport

import (
	"testing"

	"github.com/gin-gonic/gin"

	teacherpresencepkg "github.com/ghalitsar-coder/backend-ifter/internal/teacherpresence"
)

// TestRegisterTeacherPresenceRoutes is DB-free: it asserts the §6.1 contract
// routes are attached to the right capability groups and that the scaffold
// stays a no-op when the service is not wired.
func TestRegisterTeacherPresenceRoutes(t *testing.T) {
	log := discardLogger()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	v1 := engine.Group("/api/v1")
	groups := RouteGroups{
		AttendanceOps:  v1.Group("/ops"),
		AttendanceRead: v1.Group("/read"),
		Admin:          v1.Group("/admin"),
	}

	RegisterTeacherPresence(v1, groups, FeatureDeps{Log: log})
	if routes := engine.Routes(); len(routes) != 0 {
		t.Fatalf("scaffold registered %d routes without a service", len(routes))
	}

	deps := FeatureDeps{Log: log, TeacherPresence: teacherpresencepkg.NewService(nil, log)}
	ops := v1.Group("")
	read := v1.Group("")
	admin := v1.Group("")
	RegisterTeacherPresence(v1, RouteGroups{AttendanceOps: ops, AttendanceRead: read, Admin: admin}, deps)

	registered := make(map[string]struct{})
	for _, route := range engine.Routes() {
		registered[route.Method+" "+route.Path] = struct{}{}
	}
	expected := []string{
		"POST /api/v1/teacher-presence/check-in",
		"POST /api/v1/teacher-presence/check-out",
		"GET /api/v1/teacher-presence/today",
		"GET /api/v1/teacher-presence/history",
		"GET /api/v1/geofence",
		"PUT /api/v1/geofence",
	}
	for _, route := range expected {
		if _, ok := registered[route]; !ok {
			t.Errorf("teacher presence router is missing expected route: %s", route)
		}
	}
	for _, unexpected := range []string{
		"GET /api/v1/ops/teacher-presence/check-in",
		"GET /api/v1/read/teacher-presence/check-in",
	} {
		if _, ok := registered[unexpected]; ok {
			t.Errorf("group prefix leaked into a route path: %s", unexpected)
		}
	}
}
