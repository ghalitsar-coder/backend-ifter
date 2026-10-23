package httptransport

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/discipline"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

type disciplineHandler struct {
	service *discipline.Service
	log     *slog.Logger
}

type disciplineCategoryRequest struct {
	Code                   string `json:"code"`
	Name                   string `json:"name" binding:"required"`
	Description            string `json:"description"`
	DefaultPoints          *int   `json:"default_points"`
	AutoOnAttendanceStatus string `json:"auto_on_attendance_status"`
}

type disciplineCategoryStatusRequest struct {
	IsActive *bool `json:"is_active" binding:"required"`
}

type disciplineRecordRequest struct {
	StudentID   string `json:"student_id" binding:"required"`
	CategoryID  string `json:"category_id" binding:"required"`
	OccurredAt  string `json:"occurred_at" binding:"required"`
	Description string `json:"description" binding:"required"`
}

type disciplineFollowupRequest struct {
	Status string `json:"status" binding:"required"`
	Notes  string `json:"notes"`
}

type disciplinePointsRequest struct {
	Points int32  `json:"points"`
	Reason string `json:"reason" binding:"required"`
}

func newDisciplineHandler(service *discipline.Service, log *slog.Logger) *disciplineHandler {
	return &disciplineHandler{service: service, log: log}
}

func (h *disciplineHandler) actor(c *gin.Context) (discipline.Actor, bool) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		h.writeError(c, discipline.ErrUnauthorized)
		return discipline.Actor{}, false
	}
	return discipline.Actor{ID: user.ID, Roles: user.Roles}, true
}

func (h *disciplineHandler) ListCategories(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	includeInactive := strings.EqualFold(c.Query("include_inactive"), "true")
	result, err := h.service.ListCategories(c.Request.Context(), actor, includeInactive)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"data": result})
}

func (h *disciplineHandler) CreateCategory(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request disciplineCategoryRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, discipline.ErrInvalidInput)
		return
	}
	result, err := h.service.CreateCategory(c.Request.Context(), actor, discipline.CategoryInput{
		Code:                   request.Code,
		Name:                   request.Name,
		Description:            request.Description,
		DefaultPoints:          request.DefaultPoints,
		AutoOnAttendanceStatus: request.AutoOnAttendanceStatus,
	}, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, result)
}

func (h *disciplineHandler) UpdateCategory(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request disciplineCategoryRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, discipline.ErrInvalidInput)
		return
	}
	result, err := h.service.UpdateCategory(c.Request.Context(), actor, c.Param("id"), discipline.CategoryInput{
		Code:                   request.Code,
		Name:                   request.Name,
		Description:            request.Description,
		DefaultPoints:          request.DefaultPoints,
		AutoOnAttendanceStatus: request.AutoOnAttendanceStatus,
	}, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

func (h *disciplineHandler) SetCategoryStatus(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request disciplineCategoryStatusRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.IsActive == nil {
		h.writeError(c, discipline.ErrInvalidInput)
		return
	}
	result, err := h.service.SetCategoryStatus(c.Request.Context(), actor, c.Param("id"), *request.IsActive, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

func (h *disciplineHandler) ListRecords(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	limit, offset, err := parseDisciplinePage(c)
	if err != nil {
		h.writeError(c, discipline.ErrInvalidInput)
		return
	}
	result, err := h.service.List(c.Request.Context(), actor, discipline.RecordFilter{
		From:       c.Query("from"),
		To:         c.Query("to"),
		StudentID:  c.Query("student_id"),
		ClassID:    c.Query("class_id"),
		CategoryID: c.Query("category_id"),
		RecordedBy: c.Query("recorded_by"),
		Status:     c.Query("status"),
		Limit:      limit,
		Offset:     offset,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

func (h *disciplineHandler) Report(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	result, err := h.service.Report(c.Request.Context(), actor, discipline.ReportFilter{
		From:       c.Query("from"),
		To:         c.Query("to"),
		TermID:     c.Query("term_id"),
		StudentID:  c.Query("student_id"),
		ClassID:    c.Query("class_id"),
		CategoryID: c.Query("category_id"),
		RecordedBy: c.Query("recorded_by"),
		Status:     c.Query("status"),
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

func (h *disciplineHandler) CreateRecord(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request disciplineRecordRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, discipline.ErrInvalidInput)
		return
	}
	result, err := h.service.CreateRecord(c.Request.Context(), actor, discipline.RecordInput{
		StudentID:   request.StudentID,
		CategoryID:  request.CategoryID,
		OccurredAt:  request.OccurredAt,
		Description: request.Description,
	}, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, result)
}

func (h *disciplineHandler) GetRecord(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	result, err := h.service.GetRecord(c.Request.Context(), actor, c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

func (h *disciplineHandler) AddFollowup(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request disciplineFollowupRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, discipline.ErrInvalidInput)
		return
	}
	result, err := h.service.AddFollowup(c.Request.Context(), actor, c.Param("id"), discipline.FollowupInput{
		Status: request.Status,
		Notes:  request.Notes,
	}, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

func (h *disciplineHandler) UpdatePoints(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request disciplinePointsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, discipline.ErrInvalidInput)
		return
	}
	result, err := h.service.UpdatePoints(c.Request.Context(), actor, c.Param("id"), discipline.PointsInput{
		Points: request.Points,
		Reason: request.Reason,
	}, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

func (h *disciplineHandler) StudentSummary(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	result, err := h.service.StudentSummary(c.Request.Context(), actor, c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

func (h *disciplineHandler) Export(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	content, err := h.service.Export(c.Request.Context(), actor, discipline.RecordFilter{
		From:       c.Query("from"),
		To:         c.Query("to"),
		StudentID:  c.Query("student_id"),
		ClassID:    c.Query("class_id"),
		CategoryID: c.Query("category_id"),
		RecordedBy: c.Query("recorded_by"),
		Status:     c.Query("status"),
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	filename := "laporan-kedisiplinan-" + c.Query("from") + "-" + c.Query("to") + ".xlsx"
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", content)
}

func (h *disciplineHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, discipline.ErrUnauthorized):
		c.Header("WWW-Authenticate", "Bearer")
		response.Error(c, http.StatusUnauthorized, "unauthorized", "authentication required")
	case errors.Is(err, discipline.ErrForbidden):
		response.Error(c, http.StatusForbidden, "forbidden", "insufficient permissions")
	case errors.Is(err, discipline.ErrInvalidInput):
		response.Error(c, http.StatusBadRequest, "invalid_request", "invalid request")
	case errors.Is(err, discipline.ErrNotFound):
		response.Error(c, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, discipline.ErrConflict):
		response.Error(c, http.StatusConflict, "conflict", "discipline data conflicts with an existing record")
	default:
		h.log.Error("discipline_error", "request_id", response.RequestID(c), "error", err)
		response.Error(c, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func parseDisciplinePage(c *gin.Context) (int32, int32, error) {
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
