package httptransport

import (
	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
)

// RegisterDevices wires the device registry and RFID tap ingest (F10/F11).
// Devices are deactivated, never deleted, so tap data keeps a stable reference.
//
//	groups.Admin (admin):
//	  GET    /api/v1/devices?status=&type=  -> { "data": [...] }
//	  POST   /api/v1/devices                -> 201 { "device": {...}, "ingest_token": "..." }
//	  GET    /api/v1/devices/:id            -> { "device": {...} }
//	  PUT    /api/v1/devices/:id            -> { "device": {...} }
//	  PATCH  /api/v1/devices/:id/status     -> { "device": {...} }
//	  POST   /api/v1/devices/:id/token      -> { "ingest_token": "..." }
//
//	groups.MasterData (admin, staff_tu):
//	  GET    /api/v1/students/:id/card      -> { "student_card": {...} }
//	  PUT    /api/v1/students/:id/card      -> { "student_card": {...} }
//
//	v1 (device token in X-Device-Token):
//	  POST   /api/v1/device-taps            -> { accepted, duplicates, unknown_cards }
//
//	admin + duty_teacher + principal:
//	  GET    /api/v1/device-taps?device_id=&student_id=&card_uid=&from=&to=&limit=&offset=
//
// Writes are audited as device.create / device.update / device.status /
// device.token / student.card.
func RegisterDevices(v1 *gin.RouterGroup, groups RouteGroups, deps FeatureDeps) {
	if deps.Devices == nil {
		return
	}
	handler := newDeviceHandler(deps.Devices, deps.Log)

	groups.Admin.GET("/devices", handler.List)
	groups.Admin.POST("/devices", middleware.BodyLimit(16<<10), handler.Create)
	groups.Admin.GET("/devices/:id", handler.Get)
	groups.Admin.PUT("/devices/:id", middleware.BodyLimit(16<<10), handler.Update)
	groups.Admin.PATCH("/devices/:id/status", middleware.BodyLimit(4<<10), handler.SetStatus)
	groups.Admin.POST("/devices/:id/token", handler.RotateToken)

	groups.MasterData.GET("/students/:id/card", handler.GetStudentCard)
	groups.MasterData.PUT("/students/:id/card", middleware.BodyLimit(4<<10), handler.SetStudentCard)

	v1.POST("/device-taps", middleware.BodyLimit(128<<10), handler.IngestTaps)

	tapReaders := groups.Authenticated.Group("")
	tapReaders.Use(middleware.RequireRoles(RoleAdmin, RoleDutyTeacher, RolePrincipal))
	tapReaders.GET("/device-taps", handler.ListTaps)
}
