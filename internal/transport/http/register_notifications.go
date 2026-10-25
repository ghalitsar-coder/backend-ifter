package httptransport

import (
	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
)

// RegisterNotifications wires browser Web Push (F9). Every signed-in user can
// manage their own subscriptions; the delivery to admin/principal subscribers
// on a new discipline record is driven from the discipline handler.
//
//	groups.Authenticated (any signed-in user):
//	  GET    /api/v1/push/config          -> { "enabled": bool, "public_key": string }
//	  GET    /api/v1/push/subscriptions   -> { "data": [...] }
//	  POST   /api/v1/push/subscriptions   -> 201 { "subscription": {...} }
//	  DELETE /api/v1/push/subscriptions/:id -> 204
//
//	groups.Admin (admin):
//	  POST   /api/v1/admin/push/test      -> { "sent": n, "failed": n }
//
// Subscription writes and test sends answer 503 push_disabled until the
// IFTER_WEBPUSH_* VAPID configuration is complete.
func RegisterNotifications(v1 *gin.RouterGroup, groups RouteGroups, deps FeatureDeps) {
	_ = v1
	if deps.Notifications == nil {
		return
	}
	handler := newPushHandler(deps.Notifications, deps.Log)

	groups.Authenticated.GET("/push/config", handler.Config)
	groups.Authenticated.GET("/push/subscriptions", handler.List)
	groups.Authenticated.POST("/push/subscriptions", middleware.BodyLimit(16<<10), handler.Subscribe)
	groups.Authenticated.DELETE("/push/subscriptions/:id", handler.Unsubscribe)

	groups.Admin.POST("/admin/push/test", middleware.BodyLimit(8<<10), handler.SendTest)
}
