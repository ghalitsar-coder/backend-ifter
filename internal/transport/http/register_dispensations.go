package httptransport

import (
	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
)

// RegisterDispensations wires dispensation (permits/excuses) management and the
// student's own dispensation requests. See docs/API_ADDENDUM.md §6.2.
//
// Owner: dispensations backend agent (migration 000010).
//
//	groups.DispensationsRead (admin + staff_tu + teacher + duty_teacher + principal)
//	  GET  /api/v1/dispensations?status=&type=&student_id=&class_id=&from=&to=&limit=&offset=
//	  GET  /api/v1/dispensations/:id
//	  GET  /api/v1/dispensations/:id/document   (private file download, nosniff)
//
//	groups.AttendanceOps (admin + teacher + duty_teacher) — verify/decision; teacher
//	  is scoped to own class, duty_teacher school-wide (server-side scoping).
//	  POST /api/v1/dispensations/:id/verify  { status, note? }  (audited)
//
//	Staff create (admin + staff_tu + teacher + duty_teacher, per the package spec):
//	no capability group carries exactly that role set, so the create route runs on a
//	dedicated RequireRoles sub-group of the authenticated parent.
//	  POST /api/v1/dispensations  (multipart fields student_id,type,reason,start_date,
//	                              end_date + optional file part "document", or JSON)
//
//	groups.My (student only) — self-service, always scoped to the signed-in student
//	via students.user_id (link added by migration 000015; 403 until then):
//	  GET  /api/v1/my/dispensations?status=&limit=&offset=
//	  POST /api/v1/my/dispensations  (multipart or JSON; never a client student id)
func RegisterDispensations(v1 *gin.RouterGroup, groups RouteGroups, deps FeatureDeps) {
	_ = v1
	if deps.Dispensations == nil {
		return
	}
	handler := newDispensationHandler(deps.Dispensations, deps.Log)
	uploadLimit := deps.Dispensations.MaxFileSize() + (1 << 20)

	groups.DispensationsRead.GET("/dispensations", handler.List)
	groups.DispensationsRead.GET("/dispensations/:id", handler.Get)
	groups.DispensationsRead.GET("/dispensations/:id/document", handler.Document)

	groups.AttendanceOps.POST("/dispensations/:id/verify", middleware.BodyLimit(64<<10), handler.Verify)

	staffCreates := groups.Authenticated.Group("")
	staffCreates.Use(middleware.RequireRoles(RoleAdmin, RoleStaffTU, RoleTeacher, RoleDutyTeacher))
	staffCreates.POST("/dispensations", middleware.BodyLimit(uploadLimit), handler.Create)

	groups.My.GET("/my/dispensations", handler.ListMine)
	groups.My.POST("/my/dispensations", middleware.BodyLimit(uploadLimit), handler.CreateMine)
}
