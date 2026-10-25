package httptransport

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/teacherpresence"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

type teacherPresenceHandler struct {
	service *teacherpresence.Service
	log     *slog.Logger
}

// teacherPresenceCheckRequest uses pointers for the coordinates because a
// legitimate fix may report 0.0 (equator / Greenwich meridian), which gin's
// `required` tag would wrongly reject.
type teacherPresenceCheckRequest struct {
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
	Accuracy  float64  `json:"accuracy"`
	CheckTime string   `json:"check_time"`
	Notes     string   `json:"notes"`
}

type geofenceUpdateRequest struct {
	Latitude     *float64 `json:"latitude"`
	Longitude    *float64 `json:"longitude"`
	RadiusMeters *int32   `json:"radius_meters"`
	IsEnabled    *bool    `json:"is_enabled"`
}

func newTeacherPresenceHandler(service *teacherpresence.Service, log *slog.Logger) *teacherPresenceHandler {
	return &teacherPresenceHandler{service: service, log: log}
}

func (h *teacherPresenceHandler) actor(c *gin.Context) (teacherpresence.Actor, bool) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		h.writeError(c, teacherpresence.ErrUnauthorized)
		return teacherpresence.Actor{}, false
	}
	return teacherpresence.Actor{ID: user.ID, Roles: user.Roles}, true
}

func (h *teacherPresenceHandler) CheckIn(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request teacherPresenceCheckRequest
	input, err := bindCheckRequest(c, &request)
	if err != nil {
		h.writeError(c, err)
		return
	}
	result, err := h.service.CheckIn(c.Request.Context(), actor, input)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, result)
}

func (h *teacherPresenceHandler) CheckOut(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request teacherPresenceCheckRequest
	input, err := bindCheckRequest(c, &request)
	if err != nil {
		h.writeError(c, err)
		return
	}
	result, err := h.service.CheckOut(c.Request.Context(), actor, input)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, result)
}

func (h *teacherPresenceHandler) Today(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	result, err := h.service.Today(c.Request.Context(), actor)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

func (h *teacherPresenceHandler) History(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	limit, offset, err := parseTeacherPresencePage(c)
	if err != nil {
		h.writeError(c, teacherpresence.ErrInvalidInput)
		return
	}
	result, err := h.service.History(c.Request.Context(), actor, teacherpresence.HistoryFilter{
		From:   strings.TrimSpace(c.Query("from")),
		To:     strings.TrimSpace(c.Query("to")),
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

func (h *teacherPresenceHandler) GetGeofence(c *gin.Context) {
	result, err := h.service.GetGeofence(c.Request.Context())
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

func (h *teacherPresenceHandler) UpdateGeofence(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request geofenceUpdateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, teacherpresence.ErrInvalidInput)
		return
	}
	if request.Latitude == nil || request.Longitude == nil || request.RadiusMeters == nil || request.IsEnabled == nil {
		h.writeError(c, teacherpresence.ErrInvalidInput)
		return
	}
	result, err := h.service.UpdateGeofence(c.Request.Context(), actor, teacherpresence.GeofenceInput{
		Latitude:     *request.Latitude,
		Longitude:    *request.Longitude,
		RadiusMeters: *request.RadiusMeters,
		IsEnabled:    *request.IsEnabled,
	}, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

func bindCheckRequest(c *gin.Context, request *teacherPresenceCheckRequest) (teacherpresence.CheckInput, error) {
	if err := c.ShouldBindJSON(request); err != nil {
		return teacherpresence.CheckInput{}, teacherpresence.ErrInvalidInput
	}
	if request.Latitude == nil || request.Longitude == nil {
		return teacherpresence.CheckInput{}, teacherpresence.ErrInvalidInput
	}
	return teacherpresence.CheckInput{
		Latitude:  *request.Latitude,
		Longitude: *request.Longitude,
		Accuracy:  request.Accuracy,
		CheckTime: request.CheckTime,
		Notes:     request.Notes,
	}, nil
}

func parseTeacherPresencePage(c *gin.Context) (int32, int32, error) {
	limit := int32(50)
	offset := int32(0)
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || value < 1 || value > 100 {
			return 0, 0, errors.New("invalid limit")
		}
		limit = int32(value)
	}
	if raw := strings.TrimSpace(c.Query("offset")); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || value < 0 {
			return 0, 0, errors.New("invalid offset")
		}
		offset = int32(value)
	}
	return limit, offset, nil
}

func (h *teacherPresenceHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, teacherpresence.ErrUnauthorized):
		c.Header("WWW-Authenticate", "Bearer")
		response.Error(c, http.StatusUnauthorized, "unauthorized", "authentication required")
	case errors.Is(err, teacherpresence.ErrForbidden):
		response.Error(c, http.StatusForbidden, "forbidden", "insufficient permissions")
	case errors.Is(err, teacherpresence.ErrInvalidInput):
		response.Error(c, http.StatusBadRequest, "invalid_request", "invalid request")
	case errors.Is(err, teacherpresence.ErrNotFound):
		response.Error(c, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, teacherpresence.ErrConflict):
		response.Error(c, http.StatusConflict, "conflict", "teacher presence conflicts with an existing record")
	default:
		h.log.Error("teacher_presence_error", "request_id", response.RequestID(c), "error", err)
		response.Error(c, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
