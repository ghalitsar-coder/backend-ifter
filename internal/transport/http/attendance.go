package httptransport

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/attendance"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

type attendanceHandler struct {
	service *attendance.Service
	log     *slog.Logger
}

func newAttendanceHandler(service *attendance.Service, log *slog.Logger) *attendanceHandler {
	return &attendanceHandler{service: service, log: log}
}

type createSessionRequest struct {
	ClassID        int64  `json:"class_id" binding:"required"`
	ClassSubjectID *int64 `json:"class_subject_id"`
	Tanggal        string `json:"tanggal" binding:"required"`
}

func (h *attendanceHandler) CreateSession(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}

	var req createSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "invalid_request", "class_id and tanggal (YYYY-MM-DD) are required")
		return
	}

	created, err := h.service.CreateSession(c.Request.Context(), actor, attendance.CreateSessionInput{
		ClassID:        req.ClassID,
		ClassSubjectID: req.ClassSubjectID,
		Tanggal:        req.Tanggal,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, created)
}

func (h *attendanceHandler) GetSession(c *gin.Context) {
	id, ok := sessionID(c)
	if !ok {
		return
	}

	detail, err := h.service.GetSession(c.Request.Context(), id)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, detail)
}

type recordRequest struct {
	Attendances []attendance.AttendanceInput `json:"attendances" binding:"required"`
}

func (h *attendanceHandler) Record(c *gin.Context) {
	id, ok := sessionID(c)
	if !ok {
		return
	}
	actor, ok := h.actor(c)
	if !ok {
		return
	}

	var req recordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "invalid_request", "attendances must be an array")
		return
	}

	detail, err := h.service.Record(c.Request.Context(), actor, attendance.RecordInput{
		SessionID:   id,
		Attendances: req.Attendances,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, detail)
}

func (h *attendanceHandler) Lock(c *gin.Context) {
	id, ok := sessionID(c)
	if !ok {
		return
	}
	actor, ok := h.actor(c)
	if !ok {
		return
	}

	locked, err := h.service.Lock(c.Request.Context(), actor, id)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, locked)
}

func (h *attendanceHandler) actor(c *gin.Context) (attendance.Actor, bool) {
	claims, ok := middleware.CurrentClaims(c)
	if !ok {
		response.Error(c, http.StatusUnauthorized, "unauthorized", "authentication required")
		return attendance.Actor{}, false
	}
	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || userID <= 0 {
		response.Error(c, http.StatusForbidden, "invalid_subject", "token subject must be a user id")
		return attendance.Actor{}, false
	}

	manage := false
	for _, role := range claims.Roles {
		if role == "tu" || role == "admin" {
			manage = true
			break
		}
	}
	return attendance.Actor{UserID: userID, Manage: manage}, true
}

func (h *attendanceHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, attendance.ErrNotFound):
		response.Error(c, http.StatusNotFound, "not_found", "attendance session not found")
	case errors.Is(err, attendance.ErrNotStaff):
		response.Error(c, http.StatusForbidden, "not_staff", "token subject is not an active staff account")
	case errors.Is(err, attendance.ErrForbidden):
		response.Error(c, http.StatusForbidden, "forbidden", "not allowed to record attendance for this class")
	case errors.Is(err, attendance.ErrLocked):
		response.Error(c, http.StatusConflict, "locked", "attendance session is locked")
	case errors.Is(err, attendance.ErrInvalidClass):
		response.Error(c, http.StatusUnprocessableEntity, "invalid_class", "class_id is required")
	case errors.Is(err, attendance.ErrInvalidSubject):
		response.Error(c, http.StatusUnprocessableEntity, "invalid_subject", "class_subject_id must be positive")
	case errors.Is(err, attendance.ErrInvalidDate):
		response.Error(c, http.StatusUnprocessableEntity, "invalid_date", "tanggal must be YYYY-MM-DD")
	case errors.Is(err, attendance.ErrInvalidStudent):
		response.Error(c, http.StatusUnprocessableEntity, "invalid_student", "student_id must be positive")
	case errors.Is(err, attendance.ErrInvalidStatus):
		response.Error(c, http.StatusUnprocessableEntity, "invalid_status", "status must be hadir, sakit, izin, or alpa")
	case errors.Is(err, attendance.ErrEmptyAttendances):
		response.Error(c, http.StatusUnprocessableEntity, "empty_attendances", "attendances must not be empty")
	case errors.Is(err, attendance.ErrDuplicateStudent):
		response.Error(c, http.StatusUnprocessableEntity, "duplicate_student", "each student may appear only once")
	case errors.Is(err, attendance.ErrStudentNotEnrolled):
		response.Error(c, http.StatusUnprocessableEntity, "student_not_enrolled", "every student must be actively enrolled in the class")
	default:
		h.log.Error("attendance_request_failed", "request_id", response.RequestID(c), "error", err)
		response.Error(c, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func sessionID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, http.StatusBadRequest, "invalid_id", "id must be a positive integer")
		return 0, false
	}
	return id, true
}
