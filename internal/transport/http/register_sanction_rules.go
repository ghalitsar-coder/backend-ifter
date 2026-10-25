package httptransport

import (
	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
)

// RegisterSanctionRules wires the sanction-rule configuration surface — the
// thresholds that map accumulated discipline points to a sanction/tier.
// See docs/API_ADDENDUM.md §4/§6.4 (external shape is authoritative) and the
// sanction-rules package spec (migration 000012).
//
// Owner: sanction-rules backend agent.
//
// This is an admin-only new write surface (PRD §2.1 + this wave). Reads are
// allowed for every staff role that sees discipline reports.
//
//	groups.DisciplineRead (admin + teacher + duty_teacher + principal)
//	  GET    /api/v1/sanction-rules           -> { "data": [...] }
//	  GET    /api/v1/sanction-rules/:id       -> { "sanction_rule": {...} }
//
//	groups.Admin (admin) — every write audited (sanction_rule.*):
//	  POST   /api/v1/sanction-rules            { name, min_points, max_points?, action, level?, description? }
//	  PUT    /api/v1/sanction-rules            { rules: [{min_points, max_points?, action, level?, is_active}] }
//	    (replace-all in one transaction, audited with before/after rule counts —
//	     package spec; the ADDENDUM reserves no collection PUT, so this is the
//	     spec's contract surface at the ADDENDUM's base path)
//	  PUT    /api/v1/sanction-rules/:id         { ... }
//	  PATCH  /api/v1/sanction-rules/:id/status  { is_active }
//	  DELETE /api/v1/sanction-rules/:id         -> 204
//
// Overlapping point ranges are rejected (409) in the use case, and
// GET /discipline/students/:id now surfaces current_level from the matching
// rule (internal/discipline).
func RegisterSanctionRules(v1 *gin.RouterGroup, groups RouteGroups, deps FeatureDeps) {
	_ = v1
	if deps.SanctionRules == nil {
		return
	}
	handler := newSanctionRuleHandler(deps.SanctionRules, deps.Log)

	groups.DisciplineRead.GET("/sanction-rules", handler.List)
	groups.DisciplineRead.GET("/sanction-rules/:id", handler.Get)

	groups.Admin.POST("/sanction-rules", middleware.BodyLimit(64<<10), handler.Create)
	groups.Admin.PUT("/sanction-rules", middleware.BodyLimit(64<<10), handler.ReplaceAll)
	groups.Admin.PUT("/sanction-rules/:id", middleware.BodyLimit(64<<10), handler.Update)
	groups.Admin.PATCH("/sanction-rules/:id/status", middleware.BodyLimit(64<<10), handler.SetStatus)
	groups.Admin.DELETE("/sanction-rules/:id", handler.Delete)
}
