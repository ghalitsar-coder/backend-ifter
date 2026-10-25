package httptransport

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
)

// RegisterParentPortal wires the guardian surface (migration 000019) on top of
// the student-accounts service:
//
//	groups.Admin  GET /users/:id/children          { "data": [...] }
//	groups.Admin  PUT /users/:id/children          { "children": [{student_id, relationship?}] }
//	groups.Parent GET /parent/children
//	groups.Parent GET /parent/children/:id/attendance
//	groups.Parent GET /parent/children/:id/attendance-summary
//	groups.Parent GET /parent/children/:id/discipline
//	groups.Parent GET /parent/children/:id/discipline-summary
//	groups.Parent GET /parent/children/:id/dispensations
//
// It reuses the same handler/service as the /my/* portal; without a wired
// student-accounts service (fake readiness or register tests) the surface stays
// disabled, like every other pool-backed feature.
func RegisterParentPortal(v1 *gin.RouterGroup, groups RouteGroups, deps FeatureDeps) {
	_ = v1
	if groups.Parent == nil || groups.Admin == nil {
		return
	}
	service := deps.StudentAccounts
	if service == nil {
		return
	}
	log := deps.Log
	if log == nil {
		log = slog.Default()
	}
	handler := newStudentAccountHandler(service, log)

	groups.Admin.GET("/users/:id/children", handler.AdminListChildren)
	groups.Admin.PUT("/users/:id/children", middleware.BodyLimit(64<<10), handler.AdminSetChildren)

	groups.Parent.GET("/parent/children", handler.ParentChildren)
	groups.Parent.GET("/parent/children/:id/attendance", handler.ParentAttendance)
	groups.Parent.GET("/parent/children/:id/attendance-summary", handler.ParentAttendanceSummary)
	groups.Parent.GET("/parent/children/:id/discipline", handler.ParentDiscipline)
	groups.Parent.GET("/parent/children/:id/discipline-summary", handler.ParentDisciplineSummary)
	groups.Parent.GET("/parent/children/:id/dispensations", handler.ParentDispensations)
}
