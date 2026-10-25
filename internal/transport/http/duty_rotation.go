package httptransport

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	dutyrotationpkg "github.com/ghalitsar-coder/backend-ifter/internal/dutyrotation"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

type dutyRotationHandler struct {
	service *dutyrotationpkg.Service
	log     *slog.Logger
}

func newDutyRotationHandler(service *dutyrotationpkg.Service, log *slog.Logger) *dutyRotationHandler {
	return &dutyRotationHandler{service: service, log: log}
}

type dutyRotationCreateRequest struct {
	DutyAssignmentID    string `json:"duty_assignment_id"`
	SubstituteTeacherID string `json:"substitute_teacher_id"`
	Date                string `json:"date"`
	Reason              string `json:"reason"`
}

type dutyRotationCancelRequest struct {
	Reason string `json:"reason"`
}

type uniformReportCreateRequest struct {
	ForDate         string  `json:"for_date"`
	TeacherID       string  `json:"teacher_id"`
	Scope           string  `json:"scope"`
	TargetStudentID *string `json:"target_student_id"`
	Note            *string `json:"note"`
}

func (h *dutyRotationHandler) actor(c *gin.Context) (dutyrotationpkg.Actor, bool) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		h.writeError(c, dutyrotationpkg.ErrUnauthorized)
		return dutyrotationpkg.Actor{}, false
	}
	return dutyrotationpkg.Actor{ID: user.ID, Roles: user.Roles}, true
}

// ListRotations handles GET /duty-rotations?date=&weekday=. Teachers are
// scoped server-side to substitutions relevant to them; admin sees all.
func (h *dutyRotationHandler) ListRotations(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	weekday := 0
	if raw := strings.TrimSpace(c.Query("weekday")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 7 {
			h.writeError(c, dutyrotationpkg.ErrInvalidInput)
			return
		}
		weekday = value
	}
	result, err := h.service.List(c.Request.Context(), actor, dutyrotationpkg.RotationFilter{
		Date:    strings.TrimSpace(c.Query("date")),
		Weekday: weekday,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

// CreateRotation handles POST /duty-rotations -> 201 { "duty_rotation": {...} },
// audited as duty.substitution.
func (h *dutyRotationHandler) CreateRotation(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request dutyRotationCreateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, dutyrotationpkg.ErrInvalidInput)
		return
	}
	item, err := h.service.Create(c.Request.Context(), actor, dutyrotationpkg.CreateRotationInput{
		DutyAssignmentID:    strings.TrimSpace(request.DutyAssignmentID),
		SubstituteTeacherID: strings.TrimSpace(request.SubstituteTeacherID),
		Date:                strings.TrimSpace(request.Date),
		Reason:              request.Reason,
	}, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, gin.H{"duty_rotation": item})
}

// CancelRotation handles PATCH /duty-rotations/:id/cancel { reason? } ->
// 200 { "duty_rotation": {...} }, audited as duty.substitution.cancel.
func (h *dutyRotationHandler) CancelRotation(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request dutyRotationCancelRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, dutyrotationpkg.ErrInvalidInput)
		return
	}
	item, err := h.service.Cancel(c.Request.Context(), actor, c.Param("id"), dutyrotationpkg.CancelInput{
		Reason: request.Reason,
	}, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"duty_rotation": item})
}

// CreateUniform handles POST /uniform-reports -> 201 { "uniform_report": {...} }.
func (h *dutyRotationHandler) CreateUniform(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request uniformReportCreateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, dutyrotationpkg.ErrInvalidInput)
		return
	}
	input := dutyrotationpkg.CreateUniformInput{
		ForDate:   strings.TrimSpace(request.ForDate),
		TeacherID: strings.TrimSpace(request.TeacherID),
		Scope:     strings.TrimSpace(request.Scope),
	}
	if request.TargetStudentID != nil {
		input.TargetStudentID = strings.TrimSpace(*request.TargetStudentID)
	}
	if request.Note != nil {
		input.Note = *request.Note
	}
	item, err := h.service.CreateUniform(c.Request.Context(), actor, input)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, gin.H{"uniform_report": item})
}

// ListUniforms handles GET /uniform-reports?date&teacher_id.
func (h *dutyRotationHandler) ListUniforms(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	result, err := h.service.ListUniform(c.Request.Context(), actor, dutyrotationpkg.UniformFilter{
		Date:      strings.TrimSpace(c.Query("date")),
		TeacherID: strings.TrimSpace(c.Query("teacher_id")),
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

func (h *dutyRotationHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, dutyrotationpkg.ErrUnauthorized):
		c.Header("WWW-Authenticate", "Bearer")
		response.Error(c, http.StatusUnauthorized, "unauthorized", "authentication required")
	case errors.Is(err, dutyrotationpkg.ErrForbidden):
		response.Error(c, http.StatusForbidden, "forbidden", "insufficient permissions")
	case errors.Is(err, dutyrotationpkg.ErrInvalidInput):
		response.Error(c, http.StatusBadRequest, "invalid_request", "invalid request")
	case errors.Is(err, dutyrotationpkg.ErrNotFound):
		response.Error(c, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, dutyrotationpkg.ErrConflict):
		response.Error(c, http.StatusConflict, "conflict", "rotation conflicts with an existing record")
	default:
		h.log.Error("duty_rotation_error", "request_id", response.RequestID(c), "error", err)
		response.Error(c, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
