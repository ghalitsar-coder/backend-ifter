package httptransport

import (
	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
)

// RegisterTeacherPresence wires the teacher self check-in/check-out surface
// (geofenced presence). See docs/API_ADDENDUM.md §6.1.
//
// Owner: teacher-presence backend agent (migration 000009).
//
// Routes on groups.AttendanceOps (admin + teacher + duty_teacher): a user only
// ever records their own presence — the use case resolves teachers.user_id to
// the actor and returns 403 otherwise. Haversine geofence validation runs
// server-side (TECH_STACK.md §7); a client verdict is never trusted.
//
// Routes on groups.AttendanceRead add principal (read-only). The geofence
// write surface (PUT /geofence) is admin-only and audited.
func RegisterTeacherPresence(v1 *gin.RouterGroup, groups RouteGroups, deps FeatureDeps) {
	_ = v1
	if deps.TeacherPresence == nil {
		return
	}
	handler := newTeacherPresenceHandler(deps.TeacherPresence, deps.Log)

	groups.AttendanceOps.POST("/teacher-presence/check-in", middleware.BodyLimit(64<<10), handler.CheckIn)
	groups.AttendanceOps.POST("/teacher-presence/check-out", middleware.BodyLimit(64<<10), handler.CheckOut)
	groups.AttendanceRead.GET("/teacher-presence/today", handler.Today)
	groups.AttendanceRead.GET("/teacher-presence/history", handler.History)
	groups.AttendanceRead.GET("/geofence", handler.GetGeofence)
	groups.Admin.PUT("/geofence", middleware.BodyLimit(64<<10), handler.UpdateGeofence)
}
