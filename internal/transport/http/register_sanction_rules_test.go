package httptransport

import (
	"testing"

	"github.com/gin-gonic/gin"

	sanctionrulespkg "github.com/ghalitsar-coder/backend-ifter/internal/sanctionrules"
)

// TestRegisterSanctionRulesRoutes is DB-free: it asserts the §6.4 contract
// routes (plus the package spec's replace-all PUT) are attached to the right
// capability groups and that the scaffold stays a no-op when the service is
// not wired.
func TestRegisterSanctionRulesRoutes(t *testing.T) {
	log := discardLogger()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	v1 := engine.Group("/api/v1")
	groups := RouteGroups{
		Admin:          v1.Group("/admin"),
		DisciplineRead: v1.Group("/read"),
	}

	RegisterSanctionRules(v1, groups, FeatureDeps{Log: log})
	if routes := engine.Routes(); len(routes) != 0 {
		t.Fatalf("scaffold registered %d routes without a service", len(routes))
	}

	deps := FeatureDeps{Log: log, SanctionRules: sanctionrulespkg.NewService(nil, log)}
	admin := v1.Group("")
	read := v1.Group("")
	RegisterSanctionRules(v1, RouteGroups{Admin: admin, DisciplineRead: read}, deps)

	registered := make(map[string]struct{})
	for _, route := range engine.Routes() {
		registered[route.Method+" "+route.Path] = struct{}{}
	}
	expected := []string{
		"GET /api/v1/sanction-rules",
		"GET /api/v1/sanction-rules/:id",
		"POST /api/v1/sanction-rules",
		"PUT /api/v1/sanction-rules",
		"PUT /api/v1/sanction-rules/:id",
		"PATCH /api/v1/sanction-rules/:id/status",
		"DELETE /api/v1/sanction-rules/:id",
	}
	for _, route := range expected {
		if _, ok := registered[route]; !ok {
			t.Errorf("sanction-rules router is missing expected route: %s", route)
		}
	}
	for _, unexpected := range []string{
		"GET /api/v1/admin/sanction-rules",
		"GET /api/v1/read/sanction-rules",
		"POST /api/v1/read/sanction-rules",
		"PUT /api/v1/sanction-rules/:id/status",
		"DELETE /api/v1/discipline/sanction-rules",
	} {
		if _, ok := registered[unexpected]; ok {
			t.Errorf("group prefix or wrong verb/path leaked into a route path: %s", unexpected)
		}
	}
}
