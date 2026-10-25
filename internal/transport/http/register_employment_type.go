package httptransport

import (
	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
)

// RegisterEmploymentType wires the employment-type master data (e.g. PNS,
// PPPK, honorary/contract) referenced by teacher records.
// See docs/API_ADDENDUM.md §5/§6.5 (external shape is authoritative) and the
// employment-type package spec (migration 000013).
//
// Owner: employment-type backend agent.
//
// This is master data, so it lives on the masterdata capability group
// (admin + staff_tu), consistent with students/teachers/classes.
//
//	groups.MasterData (admin + staff_tu)
//	  GET    /api/v1/employment-types            -> { "data": [...] }
//	  GET    /api/v1/employment-types/:id        -> { "employment_type": {...} }
//	  POST   /api/v1/employment-types            { code, name, description?, is_active? }   -> 201
//	  PUT    /api/v1/employment-types/:id         { code, name, description?, is_active? }   -> 200
//	  PATCH  /api/v1/employment-types/:id/status  { is_active }                               -> 200
//
// Every write is audited (employment_type.create|update|status). The same
// package also extends teachers with employment_type_id/name (masterdata),
// the teacher import template/validation/export (imports), and the dashboard
// active_teachers_breakdown (reporting).
func RegisterEmploymentType(v1 *gin.RouterGroup, groups RouteGroups, deps FeatureDeps) {
	_ = v1
	if deps.EmploymentType == nil {
		return
	}
	handler := newEmploymentTypeHandler(deps.EmploymentType, deps.Log)

	groups.MasterData.GET("/employment-types", handler.List)
	groups.MasterData.GET("/employment-types/:id", handler.Get)
	groups.MasterData.POST("/employment-types", middleware.BodyLimit(64<<10), handler.Create)
	groups.MasterData.PUT("/employment-types/:id", middleware.BodyLimit(64<<10), handler.Update)
	groups.MasterData.PATCH("/employment-types/:id/status", middleware.BodyLimit(64<<10), handler.SetStatus)
}
