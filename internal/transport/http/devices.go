package httptransport

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	devicespkg "github.com/ghalitsar-coder/backend-ifter/internal/devices"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

type deviceHandler struct {
	service *devicespkg.Service
	log     *slog.Logger
}

func newDeviceHandler(service *devicespkg.Service, log *slog.Logger) *deviceHandler {
	return &deviceHandler{service: service, log: log}
}

type deviceRequest struct {
	Name       string `json:"name"`
	Identifier string `json:"identifier"`
	DeviceType string `json:"device_type"`
	Location   string `json:"location"`
	Notes      string `json:"notes"`
}

type deviceStatusRequest struct {
	Status string `json:"status"`
}

func (in deviceRequest) toInput() devicespkg.DeviceInput {
	return devicespkg.DeviceInput{
		Name:       in.Name,
		Identifier: in.Identifier,
		DeviceType: in.DeviceType,
		Location:   in.Location,
		Notes:      in.Notes,
	}
}

func (h *deviceHandler) actor(c *gin.Context) (devicespkg.Actor, bool) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		h.writeError(c, devicespkg.ErrUnauthorized)
		return devicespkg.Actor{}, false
	}
	return devicespkg.Actor{ID: user.ID, Roles: user.Roles}, true
}

// List handles GET /devices?status=&type= -> 200 { "data": [...] }.
func (h *deviceHandler) List(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	result, err := h.service.List(c.Request.Context(), actor, devicespkg.DeviceFilter{
		Status:     strings.TrimSpace(c.Query("status")),
		DeviceType: strings.TrimSpace(c.Query("type")),
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

// Get handles GET /devices/:id -> 200 { "device": {...} }.
func (h *deviceHandler) Get(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	device, err := h.service.Get(c.Request.Context(), actor, c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"device": device})
}

// Create handles POST /devices -> 201 { "device": {...} }, audited as
// device.create.
func (h *deviceHandler) Create(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request deviceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, devicespkg.ErrInvalidInput)
		return
	}
	device, err := h.service.Create(c.Request.Context(), actor, request.toInput(), response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, gin.H{"device": device})
}

// Update handles PUT /devices/:id -> 200 { "device": {...} }, audited as
// device.update.
func (h *deviceHandler) Update(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request deviceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, devicespkg.ErrInvalidInput)
		return
	}
	device, err := h.service.Update(c.Request.Context(), actor, c.Param("id"), request.toInput(), response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"device": device})
}

// SetStatus handles PATCH /devices/:id/status -> 200 { "device": {...} },
// audited as device.status.
func (h *deviceHandler) SetStatus(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request deviceStatusRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, devicespkg.ErrInvalidInput)
		return
	}
	device, err := h.service.SetStatus(c.Request.Context(), actor, c.Param("id"), request.Status, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"device": device})
}

func (h *deviceHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, devicespkg.ErrUnauthorized):
		c.Header("WWW-Authenticate", "Bearer")
		response.Error(c, http.StatusUnauthorized, "unauthorized", "authentication required")
	case errors.Is(err, devicespkg.ErrForbidden):
		response.Error(c, http.StatusForbidden, "forbidden", "insufficient permissions")
	case errors.Is(err, devicespkg.ErrInvalidInput):
		response.Error(c, http.StatusBadRequest, "invalid_request", "invalid request")
	case errors.Is(err, devicespkg.ErrNotFound):
		response.Error(c, http.StatusNotFound, "not_found", "device not found")
	case errors.Is(err, devicespkg.ErrConflict):
		response.Error(c, http.StatusConflict, "conflict", "device identifier already exists")
	default:
		h.log.Error("device_error", "request_id", response.RequestID(c), "error", err)
		response.Error(c, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
