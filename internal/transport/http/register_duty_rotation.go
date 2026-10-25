package httptransport

import (
	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
)

// RegisterDutyRotation wires duty-teacher substitution management.
// See docs/API_ADDENDUM.md §6.3 (external shape is authoritative).
//
// Owner: duty-rotation backend agent (migration 000011).
//
// The admin master duty schedule (GET/POST /duty-assignments,
// PUT /duty-assignments/:id/end) stays in router.go under groups.Admin.
// This package adds the *substitution* surface plus the uniform-note reports:
//
//	groups.AttendanceOps (admin + teacher + duty_teacher)
//	  GET   /api/v1/duty-rotations?date=&weekday=
//	  POST  /api/v1/duty-rotations            { duty_assignment_id, substitute_teacher_id, date, reason }
//	  PATCH /api/v1/duty-rotations/:id/cancel { reason? }
//
//	Uniform reports (admin + staff_tu + duty_teacher per the package spec):
//	no capability group carries exactly that role set, so — same precedent as
//	RegisterDispensations' staff create — the routes run on a dedicated
//	RequireRoles sub-group of the authenticated parent:
//	  POST /api/v1/uniform-reports  { for_date, teacher_id, scope, target_student_id?, note? }
//	  GET  /api/v1/uniform-reports?date&teacher_id
//
// Server-side scoping lives in the service: a duty_teacher/teacher may only
// view and write substitutions relevant to them (own assignment or own as
// substitute); admin acts school-wide. Substitution writes are audited.
func RegisterDutyRotation(v1 *gin.RouterGroup, groups RouteGroups, deps FeatureDeps) {
	_ = v1
	if deps.DutyRotation == nil {
		return
	}
	handler := newDutyRotationHandler(deps.DutyRotation, deps.Log)

	groups.AttendanceOps.GET("/duty-rotations", handler.ListRotations)
	groups.AttendanceOps.POST("/duty-rotations", middleware.BodyLimit(64<<10), handler.CreateRotation)
	groups.AttendanceOps.PATCH("/duty-rotations/:id/cancel", middleware.BodyLimit(64<<10), handler.CancelRotation)

	uniform := groups.Authenticated.Group("")
	uniform.Use(middleware.RequireRoles(RoleAdmin, RoleStaffTU, RoleDutyTeacher))
	uniform.POST("/uniform-reports", middleware.BodyLimit(64<<10), handler.CreateUniform)
	uniform.GET("/uniform-reports", handler.ListUniforms)
}
