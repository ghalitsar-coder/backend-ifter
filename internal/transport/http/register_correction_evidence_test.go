package httptransport

import (
	"testing"

	"github.com/gin-gonic/gin"

	correctionevidencepkg "github.com/ghalitsar-coder/backend-ifter/internal/correctionevidence"
)

// TestRegisterCorrectionEvidenceRoutes is DB-free: it asserts the §6.6
// contract routes are attached to the attendance-ops group paths verbatim
// (no group prefixes leak) and that the register stays a no-op when no
// correction-evidence service is wired in FeatureDeps.
func TestRegisterCorrectionEvidenceRoutes(t *testing.T) {
	log := discardLogger()

	engine := gin.New()
	v1, groups := newTestFeatureGroupsOn(engine)
	RegisterCorrectionEvidence(v1, groups, FeatureDeps{Log: log})
	if routes := engine.Routes(); len(routes) != 0 {
		t.Fatalf("register produced %d routes without a service", len(routes))
	}

	service := correctionevidencepkg.NewService(nil, log, correctionevidencepkg.Config{Dir: t.TempDir()})
	engine = gin.New()
	v1, groups = newTestFeatureGroupsOn(engine)
	RegisterCorrectionEvidence(v1, groups, FeatureDeps{Log: log, CorrectionEvidence: service})

	registered := make(map[string]struct{})
	for _, route := range engine.Routes() {
		registered[route.Method+" "+route.Path] = struct{}{}
	}
	expected := []string{
		"POST /api/v1/attendance/records/:id/corrections/evidence",
		"GET /api/v1/attendance/records/:id/corrections/evidence",
		"GET /api/v1/attendance/corrections/evidence/:evidenceId/download",
		"DELETE /api/v1/attendance/corrections/evidence/:evidenceId",
	}
	for _, route := range expected {
		if _, ok := registered[route]; !ok {
			t.Errorf("correction-evidence router is missing expected route: %s", route)
		}
	}
	for _, unexpected := range []string{
		"PUT /api/v1/attendance/records/:id/corrections/evidence",
		"DELETE /api/v1/attendance/records/:id/corrections/evidence/:evidenceId",
		"GET /api/v1/attendance/corrections/evidence/:evidenceId",
	} {
		if _, ok := registered[unexpected]; ok {
			t.Errorf("unexpected route registered: %s", unexpected)
		}
	}
}

func newTestFeatureGroupsOn(engine *gin.Engine) (*gin.RouterGroup, RouteGroups) {
	v1 := engine.Group("/api/v1")
	ops := v1.Group("")
	return v1, RouteGroups{AttendanceOps: ops}
}
