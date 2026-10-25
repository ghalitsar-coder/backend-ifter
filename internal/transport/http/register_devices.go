package httptransport

import (
	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
)

// RegisterDevices wires the admin-only device registry (F10): RFID readers,
// gate controllers and tablets. Devices are deactivated, never deleted, so
// future tap data (F11) keeps a stable reference.
//
//	groups.Admin (admin):
//	  GET    /api/v1/devices?status=&type=  -> { "data": [...] }
//	  POST   /api/v1/devices                -> 201 { "device": {...} }
//	  GET    /api/v1/devices/:id            -> { "device": {...} }
//	  PUT    /api/v1/devices/:id            -> { "device": {...} }
//	  PATCH  /api/v1/devices/:id/status     -> { "device": {...} }
//
// Writes are audited as device.create / device.update / device.status.
func RegisterDevices(v1 *gin.RouterGroup, groups RouteGroups, deps FeatureDeps) {
	_ = v1
	if deps.Devices == nil {
		return
	}
	handler := newDeviceHandler(deps.Devices, deps.Log)

	groups.Admin.GET("/devices", handler.List)
	groups.Admin.POST("/devices", middleware.BodyLimit(16<<10), handler.Create)
	groups.Admin.GET("/devices/:id", handler.Get)
	groups.Admin.PUT("/devices/:id", middleware.BodyLimit(16<<10), handler.Update)
	groups.Admin.PATCH("/devices/:id/status", middleware.BodyLimit(4<<10), handler.SetStatus)
}
