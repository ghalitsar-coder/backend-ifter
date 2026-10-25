package httptransport

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	notificationspkg "github.com/ghalitsar-coder/backend-ifter/internal/notifications"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

type pushHandler struct {
	service *notificationspkg.Service
	log     *slog.Logger
}

func newPushHandler(service *notificationspkg.Service, log *slog.Logger) *pushHandler {
	return &pushHandler{service: service, log: log}
}

type pushSubscriptionRequest struct {
	Endpoint  string `json:"endpoint"`
	P256DH    string `json:"p256dh"`
	Auth      string `json:"auth"`
	UserAgent string `json:"user_agent"`
}

type pushTestRequest struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

func (h *pushHandler) actor(c *gin.Context) (notificationspkg.Actor, bool) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		h.writeError(c, notificationspkg.ErrUnauthorized)
		return notificationspkg.Actor{}, false
	}
	return notificationspkg.Actor{ID: user.ID, Roles: user.Roles}, true
}

// Config handles GET /push/config. It always answers 200; enabled=false keeps
// the frontend from asking for notification permission.
func (h *pushHandler) Config(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if _, ok := h.actor(c); !ok {
		return
	}
	response.JSON(c, http.StatusOK, h.service.Config())
}

// List handles GET /push/subscriptions -> { "data": [...] } (own rows only).
func (h *pushHandler) List(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	items, err := h.service.List(c.Request.Context(), actor)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"data": items})
}

// Subscribe handles POST /push/subscriptions -> 201 { "subscription": {...} },
// upserting by endpoint.
func (h *pushHandler) Subscribe(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request pushSubscriptionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, notificationspkg.ErrInvalidInput)
		return
	}
	item, err := h.service.Subscribe(c.Request.Context(), actor, notificationspkg.SubscriptionInput{
		Endpoint:  request.Endpoint,
		P256DH:    request.P256DH,
		Auth:      request.Auth,
		UserAgent: request.UserAgent,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, gin.H{"subscription": item})
}

// Unsubscribe handles DELETE /push/subscriptions/:id -> 204; a row owned by
// someone else answers 404.
func (h *pushHandler) Unsubscribe(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	if err := h.service.Unsubscribe(c.Request.Context(), actor, c.Param("id")); err != nil {
		h.writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// SendTest handles POST /admin/push/test -> { "sent": n, "failed": n }.
func (h *pushHandler) SendTest(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request pushTestRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, notificationspkg.ErrInvalidInput)
		return
	}
	result, err := h.service.SendTest(c.Request.Context(), actor, notificationspkg.SendInput{
		Title: request.Title,
		Body:  request.Body,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

func (h *pushHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, notificationspkg.ErrUnauthorized):
		c.Header("WWW-Authenticate", "Bearer")
		response.Error(c, http.StatusUnauthorized, "unauthorized", "authentication required")
	case errors.Is(err, notificationspkg.ErrForbidden):
		response.Error(c, http.StatusForbidden, "forbidden", "insufficient permissions")
	case errors.Is(err, notificationspkg.ErrInvalidInput):
		response.Error(c, http.StatusBadRequest, "invalid_request", "invalid push subscription")
	case errors.Is(err, notificationspkg.ErrNotFound):
		response.Error(c, http.StatusNotFound, "not_found", "push subscription not found")
	case errors.Is(err, notificationspkg.ErrDisabled):
		response.Error(c, http.StatusServiceUnavailable, "push_disabled", "push notifications are not configured")
	default:
		h.log.Error("push_error", "request_id", response.RequestID(c), "error", err)
		response.Error(c, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
