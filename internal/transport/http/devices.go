package httptransport

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

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

type studentCardRequest struct {
	CardUID *string `json:"card_uid"`
}

type tapInputRequest struct {
	CardUID    string    `json:"card_uid"`
	OccurredAt time.Time `json:"occurred_at"`
}

type tapBatchRequest struct {
	DeviceIdentifier string            `json:"device_identifier"`
	Taps             []tapInputRequest `json:"taps"`
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

// Create handles POST /devices -> 201 { "device": {...}, "ingest_token": "..." },
// audited as device.create. The token is shown only in this response.
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
	device, token, err := h.service.Create(c.Request.Context(), actor, request.toInput(), response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, gin.H{"device": device, "ingest_token": token})
}

// RotateToken handles POST /devices/:id/token -> 200 { "ingest_token": "..." },
// audited as device.token.
func (h *deviceHandler) RotateToken(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	token, err := h.service.IssueIngestToken(c.Request.Context(), actor, c.Param("id"), response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"ingest_token": token})
}

// GetStudentCard handles GET /students/:id/card -> 200 { "student_card": {...} }.
func (h *deviceHandler) GetStudentCard(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	card, err := h.service.GetStudentCard(c.Request.Context(), actor, c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"student_card": card})
}

// SetStudentCard handles PUT /students/:id/card -> 200 { "student_card": {...} },
// audited as student.card.
func (h *deviceHandler) SetStudentCard(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request studentCardRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, devicespkg.ErrInvalidInput)
		return
	}
	card, err := h.service.SetStudentCard(c.Request.Context(), actor, c.Param("id"), request.CardUID, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"student_card": card})
}

// IngestTaps handles POST /device-taps (unauthenticated route, device token in
// X-Device-Token) -> 200 { accepted, duplicates, unknown_cards }.
func (h *deviceHandler) IngestTaps(c *gin.Context) {
	var request tapBatchRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, devicespkg.ErrInvalidInput)
		return
	}
	taps := make([]devicespkg.TapInput, 0, len(request.Taps))
	for _, tap := range request.Taps {
		taps = append(taps, devicespkg.TapInput{CardUID: tap.CardUID, OccurredAt: tap.OccurredAt})
	}
	result, err := h.service.IngestTaps(c.Request.Context(), c.GetHeader("X-Device-Token"), devicespkg.TapBatch{
		Identifier: request.DeviceIdentifier,
		Taps:       taps,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

// ListTaps handles GET /device-taps?device_id=&student_id=&card_uid=&from=&to=&limit=&offset=.
func (h *deviceHandler) ListTaps(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	limit, err := deviceInt32Query(c.Query("limit"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	offset, err := deviceInt32Query(c.Query("offset"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	result, err := h.service.ListTaps(c.Request.Context(), actor, devicespkg.TapFilter{
		DeviceID:  strings.TrimSpace(c.Query("device_id")),
		StudentID: strings.TrimSpace(c.Query("student_id")),
		CardUID:   strings.TrimSpace(c.Query("card_uid")),
		From:      strings.TrimSpace(c.Query("from")),
		To:        strings.TrimSpace(c.Query("to")),
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

func deviceInt32Query(raw string) (int32, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(trimmed, 10, 32)
	if err != nil {
		return 0, devicespkg.ErrInvalidInput
	}
	return int32(value), nil
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
	case errors.Is(err, devicespkg.ErrInvalidToken):
		c.Header("WWW-Authenticate", "Bearer")
		response.Error(c, http.StatusUnauthorized, "invalid_device_token", "invalid device token")
	case errors.Is(err, devicespkg.ErrDeviceInactive):
		response.Error(c, http.StatusForbidden, "device_inactive", "device is inactive")
	case errors.Is(err, devicespkg.ErrForbidden):
		response.Error(c, http.StatusForbidden, "forbidden", "insufficient permissions")
	case errors.Is(err, devicespkg.ErrInvalidInput):
		response.Error(c, http.StatusBadRequest, "invalid_request", "invalid request")
	case errors.Is(err, devicespkg.ErrNotFound):
		response.Error(c, http.StatusNotFound, "not_found", "device not found")
	case errors.Is(err, devicespkg.ErrStudentNotFound):
		response.Error(c, http.StatusNotFound, "not_found", "student not found")
	case errors.Is(err, devicespkg.ErrConflict):
		response.Error(c, http.StatusConflict, "conflict", "resource already exists")
	default:
		h.log.Error("device_error", "request_id", response.RequestID(c), "error", err)
		response.Error(c, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
