package httptransport

import (
	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
)

// RegisterSubjectAttendance wires per-subject attendance (migration 000020):
// subject catalog, teaching assignments, lesson sessions, per-student session
// records and the recap report.
//
//	staffRead (admin + staff_tu + teacher + duty_teacher + principal):
//	  GET  /api/v1/subjects?search=&is_active=
//	  GET  /api/v1/subjects/:id
//	  GET  /api/v1/teaching-assignments?teacher_id=&subject_id=&class_id=&is_active=
//	  GET  /api/v1/teaching-assignments/:id
//
//	groups.MasterData (admin + staff_tu) — catalog and assignment writes:
//	  POST /api/v1/subjects
//	  PUT  /api/v1/subjects/:id
//	  POST /api/v1/teaching-assignments
//	  PUT  /api/v1/teaching-assignments/:id
//
//	groups.AttendanceOps (admin + teacher + duty_teacher) — session writes:
//	  POST /api/v1/attendance-sessions
//	  PUT  /api/v1/attendance-sessions/:id/records
//	  POST /api/v1/attendance-sessions/:id/close
//
//	groups.AttendanceRead (admin + teacher + duty_teacher + principal):
//	  GET  /api/v1/attendance-sessions?from=&to=&class_id=&subject_id=&teacher_id=&status=&limit=&offset=
//	  GET  /api/v1/attendance-sessions/:id
//	  GET  /api/v1/attendance-sessions/:id/roster
//	  GET  /api/v1/attendance-sessions/:id/records
//
//	groups.ReportsRead (all staff):
//	  GET  /api/v1/reports/subject-attendance?from=&to=&class_id=&subject_id=&teacher_id=
//
// A plain teacher is scoped server-side to their own teaching assignments;
// admin, duty_teacher and (reads only) staff_tu/principal see the whole school.
func RegisterSubjectAttendance(v1 *gin.RouterGroup, groups RouteGroups, deps FeatureDeps) {
	_ = v1
	if deps.SubjectAttendance == nil {
		return
	}
	handler := newSubjectAttendanceHandler(deps.SubjectAttendance, deps.Log)

	staffRead := groups.Authenticated.Group("")
	staffRead.Use(middleware.RequireRoles(RoleAdmin, RoleStaffTU, RoleTeacher, RoleDutyTeacher, RolePrincipal))
	staffRead.GET("/subjects", handler.ListSubjects)
	staffRead.GET("/subjects/:id", handler.GetSubject)
	staffRead.GET("/teaching-assignments", handler.ListAssignments)
	staffRead.GET("/teaching-assignments/:id", handler.GetAssignment)

	groups.MasterData.POST("/subjects", middleware.BodyLimit(16<<10), handler.CreateSubject)
	groups.MasterData.PUT("/subjects/:id", middleware.BodyLimit(16<<10), handler.UpdateSubject)
	groups.MasterData.POST("/teaching-assignments", middleware.BodyLimit(16<<10), handler.CreateAssignment)
	groups.MasterData.PUT("/teaching-assignments/:id", middleware.BodyLimit(16<<10), handler.UpdateAssignment)

	groups.AttendanceOps.POST("/attendance-sessions", middleware.BodyLimit(16<<10), handler.CreateSession)
	groups.AttendanceOps.PUT("/attendance-sessions/:id/records", middleware.BodyLimit(256<<10), handler.SaveRecords)
	groups.AttendanceOps.POST("/attendance-sessions/:id/close", middleware.BodyLimit(4<<10), handler.CloseSession)

	groups.AttendanceRead.GET("/attendance-sessions", handler.ListSessions)
	groups.AttendanceRead.GET("/attendance-sessions/:id", handler.GetSession)
	groups.AttendanceRead.GET("/attendance-sessions/:id/roster", handler.Roster)
	groups.AttendanceRead.GET("/attendance-sessions/:id/records", handler.Records)

	groups.ReportsRead.GET("/reports/subject-attendance", handler.Summary)
}
