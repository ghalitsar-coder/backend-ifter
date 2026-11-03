package httptransport

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/discipline"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

type disciplineHandler struct {
	service *discipline.Service
	log     *slog.Logger
}

func newDisciplineHandler(service *discipline.Service, log *slog.Logger) *disciplineHandler {
	return &disciplineHandler{service: service, log: log}
}

func (h *disciplineHandler) actor(c *gin.Context) (discipline.Actor, bool) {
	claims, ok := middleware.CurrentClaims(c)
	if !ok || claims == nil {
		response.Error(c, http.StatusUnauthorized, "unauthorized", "missing or invalid auth token")
		return discipline.Actor{}, false
	}
	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || userID <= 0 {
		response.Error(c, http.StatusForbidden, "invalid_subject", "token subject must be a user id")
		return discipline.Actor{}, false
	}
	if claims.SchoolID <= 0 {
		response.Error(c, http.StatusForbidden, "invalid_school", "token must carry a school id")
		return discipline.Actor{}, false
	}
	return discipline.Actor{
		UserID:   userID,
		SchoolID: claims.SchoolID,
		Roles:    claims.Roles,
	}, true
}

func (h *disciplineHandler) ListViolationTypes(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	types, err := h.service.ListViolationTypes(c.Request.Context(), actor)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, types)
}

type recordViolationRequest struct {
	InspectionID    *int64  `json:"inspection_id"`
	StudentID       int64   `json:"student_id" binding:"required"`
	ViolationTypeID int64   `json:"violation_type_id" binding:"required"`
	Poin            *int32  `json:"poin"`
	Keterangan      *string `json:"keterangan"`
	TindakLanjut    *string `json:"tindak_lanjut"`
	Tanggal         string  `json:"tanggal" binding:"required"`
}

func (h *disciplineHandler) RecordViolation(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var req recordViolationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "invalid_request", "student_id, violation_type_id, and tanggal (YYYY-MM-DD) are required")
		return
	}

	created, err := h.service.RecordViolation(c.Request.Context(), actor, discipline.RecordViolationInput{
		InspectionID:    req.InspectionID,
		StudentID:       req.StudentID,
		ViolationTypeID: req.ViolationTypeID,
		Poin:            req.Poin,
		Keterangan:      req.Keterangan,
		TindakLanjut:    req.TindakLanjut,
		Tanggal:         req.Tanggal,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, created)
}

func (h *disciplineHandler) ListViolations(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}

	filter := discipline.ListViolationsFilter{}
	if val := c.Query("student_id"); val != "" {
		if id, err := strconv.ParseInt(val, 10, 64); err == nil && id > 0 {
			filter.StudentID = &id
		}
	}
	if val := c.Query("class_id"); val != "" {
		if id, err := strconv.ParseInt(val, 10, 64); err == nil && id > 0 {
			filter.ClassID = &id
		}
	}
	if kat := c.Query("kategori"); kat != "" {
		filter.Kategori = &kat
	}

	violations, err := h.service.ListViolations(c.Request.Context(), actor, filter)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, violations)
}

func (h *disciplineHandler) GetStudentSummary(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	studentID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || studentID <= 0 {
		response.Error(c, http.StatusBadRequest, "invalid_id", "student id must be a positive integer")
		return
	}

	summary, err := h.service.GetStudentSummary(c.Request.Context(), actor, studentID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, summary)
}

type updateFollowupRequest struct {
	TindakLanjut string `json:"tindak_lanjut" binding:"required"`
}

func (h *disciplineHandler) UpdateFollowup(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, http.StatusBadRequest, "invalid_id", "violation id must be a positive integer")
		return
	}

	var req updateFollowupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "invalid_request", "tindak_lanjut is required")
		return
	}

	updated, err := h.service.UpdateFollowup(c.Request.Context(), actor, id, req.TindakLanjut)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, updated)
}

type createInspectionRequest struct {
	ClassID         int64                                    `json:"class_id" binding:"required"`
	Tanggal         string                                   `json:"tanggal" binding:"required"`
	JumlahDiperiksa int32                                    `json:"jumlah_diperiksa" binding:"required"`
	Catatan         *string                                  `json:"catatan"`
	Violations      []discipline.CreateInspectionViolationItem `json:"violations"`
}

func (h *disciplineHandler) CreateInspection(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}

	var req createInspectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "invalid_request", "class_id, tanggal, and jumlah_diperiksa are required")
		return
	}

	inspection, err := h.service.CreateInspection(c.Request.Context(), actor, discipline.CreateInspectionInput{
		ClassID:         req.ClassID,
		Tanggal:         req.Tanggal,
		JumlahDiperiksa: req.JumlahDiperiksa,
		Catatan:         req.Catatan,
		Violations:      req.Violations,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, inspection)
}

func (h *disciplineHandler) ListInspections(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}

	filter := discipline.ListInspectionsFilter{}
	if val := c.Query("class_id"); val != "" {
		if id, err := strconv.ParseInt(val, 10, 64); err == nil && id > 0 {
			filter.ClassID = &id
		}
	}
	if tStr := c.Query("tanggal"); tStr != "" {
		if d, err := discipline.ParseDate(tStr); err == nil {
			filter.Tanggal = &d
		}
	}

	inspections, err := h.service.ListInspections(c.Request.Context(), actor, filter)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, inspections)
}

func (h *disciplineHandler) ListClassStudents(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	classID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || classID <= 0 {
		response.Error(c, http.StatusBadRequest, "invalid_id", "class id must be a positive integer")
		return
	}

	students, err := h.service.ListClassStudents(c.Request.Context(), actor, classID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, students)
}

func (h *disciplineHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, discipline.ErrNotFound):
		response.Error(c, http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, discipline.ErrForbidden):
		response.Error(c, http.StatusForbidden, "forbidden", "access forbidden")
	case errors.Is(err, discipline.ErrNotStaff):
		response.Error(c, http.StatusForbidden, "not_staff", "token subject is not an active staff account")
	case errors.Is(err, discipline.ErrInvalidStudent):
		response.Error(c, http.StatusUnprocessableEntity, "invalid_student", "student_id must be a valid positive integer")
	case errors.Is(err, discipline.ErrInvalidViolationType):
		response.Error(c, http.StatusUnprocessableEntity, "invalid_violation_type", "violation_type_id must be valid")
	case errors.Is(err, discipline.ErrInvalidClass):
		response.Error(c, http.StatusUnprocessableEntity, "invalid_class", "class_id must belong to your school")
	case errors.Is(err, discipline.ErrInvalidDate):
		response.Error(c, http.StatusUnprocessableEntity, "invalid_date", "tanggal must be YYYY-MM-DD")
	case errors.Is(err, discipline.ErrInvalidPoints):
		response.Error(c, http.StatusUnprocessableEntity, "invalid_points", "points must be non-negative")
	default:
		h.log.Error("discipline_request_failed", "request_id", response.RequestID(c), "error", err)
		response.Error(c, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
