package httptransport

import (
	"testing"

	"github.com/gin-gonic/gin"

	employmenttypepkg "github.com/ghalitsar-coder/backend-ifter/internal/employmenttype"
)

// TestRegisterEmploymentTypeRoutes is DB-free: it asserts the §6.5 contract
// routes are attached to /employment-types (no group prefixes leak into the
// path) and that the scaffold stays a no-op when the service is not wired.
func TestRegisterEmploymentTypeRoutes(t *testing.T) {
	log := discardLogger()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	v1 := engine.Group("/api/v1")
	groups := RouteGroups{
		MasterData: v1.Group("/master"),
		Admin:      v1.Group("/admin"),
	}

	RegisterEmploymentType(v1, groups, FeatureDeps{Log: log})
	if routes := engine.Routes(); len(routes) != 0 {
		t.Fatalf("scaffold registered %d routes without a service", len(routes))
	}

	deps := FeatureDeps{Log: log, EmploymentType: employmenttypepkg.NewService(nil, log)}
	master := v1.Group("")
	RegisterEmploymentType(v1, RouteGroups{MasterData: master}, deps)

	registered := make(map[string]struct{})
	for _, route := range engine.Routes() {
		registered[route.Method+" "+route.Path] = struct{}{}
	}
	expected := []string{
		"GET /api/v1/employment-types",
		"GET /api/v1/employment-types/:id",
		"POST /api/v1/employment-types",
		"PUT /api/v1/employment-types/:id",
		"PATCH /api/v1/employment-types/:id/status",
	}
	for _, route := range expected {
		if _, ok := registered[route]; !ok {
			t.Errorf("employment-type router is missing expected route: %s", route)
		}
	}
	for _, unexpected := range []string{
		"GET /api/v1/master/employment-types",
		"POST /api/v1/master/employment-types",
		"DELETE /api/v1/employment-types/:id",
		"GET /api/v1/admin/employment-types",
		"PATCH /api/v1/employment-types/:id",
	} {
		if _, ok := registered[unexpected]; ok {
			t.Errorf("group prefix or wrong verb/path leaked into a route path: %s", unexpected)
		}
	}
}
