package httptransport

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	subjectattendancepkg "github.com/ghalitsar-coder/backend-ifter/internal/subjectattendance"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

type subjectAttendanceHandler struct {
	service *subjectattendancepkg.Service
	log     *slog.Logger
}

func newSubjectAttendanceHandler(service *subjectattendancepkg.Service, log *slog.Logger) *subjectAttendanceHandler {
	return &subjectAttendanceHandler{service: service, log: log}
}

type subjectRequest struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsActive    *bool  `json:"is_active"`
}

type assignmentRequest struct {
	TeacherID string `json:"teacher_id"`
	SubjectID string `json:"subject_id"`
	ClassID   string `json:"class_id"`
	IsActive  *bool  `json:"is_active"`
}

type sessionRequest struct {
	TeachingAssignmentID string `json:"teaching_assignment_id"`
	SessionDate          string `json:"session_date"`
	PeriodStart          int32  `json:"period_start"`
	PeriodEnd            int32  `json:"period_end"`
	Topic                string `json:"topic"`
	Notes                string `json:"notes"`
}

type sessionRecordRequest struct {
	StudentID string `json:"student_id"`
	Status    string `json:"status"`
	Note      string `json:"note"`
}

type saveRecordsRequest struct {
	Records []sessionRecordRequest `json:"records"`
}

func (h *subjectAttendanceHandler) actor(c *gin.Context) (subjectattendancepkg.Actor, bool) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		h.writeError(c, subjectattendancepkg.ErrUnauthorized)
		return subjectattendancepkg.Actor{}, false
	}
	return subjectattendancepkg.Actor{ID: user.ID, Roles: user.Roles}, true
}

// ListSubjects handles GET /subjects?search=&is_active=.
func (h *subjectAttendanceHandler) ListSubjects(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	isActive, err := boolQuery(c.Query("is_active"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	result, err := h.service.ListSubjects(c.Request.Context(), actor, subjectattendancepkg.SubjectFilter{
		Search:   strings.TrimSpace(c.Query("search")),
		IsActive: isActive,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

// GetSubject handles GET /subjects/:id -> { "subject": {...} }.
func (h *subjectAttendanceHandler) GetSubject(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	subject, err := h.service.GetSubject(c.Request.Context(), actor, c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"subject": subject})
}

// CreateSubject handles POST /subjects -> 201 { "subject": {...} }.
func (h *subjectAttendanceHandler) CreateSubject(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request subjectRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, subjectattendancepkg.ErrInvalidInput)
		return
	}
	subject, err := h.service.CreateSubject(c.Request.Context(), actor, subjectattendancepkg.SubjectInput{
		Code:        request.Code,
		Name:        request.Name,
		Description: request.Description,
		IsActive:    request.IsActive,
	}, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, gin.H{"subject": subject})
}

// UpdateSubject handles PUT /subjects/:id -> { "subject": {...} }.
func (h *subjectAttendanceHandler) UpdateSubject(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request subjectRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, subjectattendancepkg.ErrInvalidInput)
		return
	}
	subject, err := h.service.UpdateSubject(c.Request.Context(), actor, c.Param("id"), subjectattendancepkg.SubjectInput{
		Code:        request.Code,
		Name:        request.Name,
		Description: request.Description,
		IsActive:    request.IsActive,
	}, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"subject": subject})
}

// ListAssignments handles GET /teaching-assignments?teacher_id=&subject_id=&class_id=&is_active=.
func (h *subjectAttendanceHandler) ListAssignments(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	isActive, err := boolQuery(c.Query("is_active"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	result, err := h.service.ListTeachingAssignments(c.Request.Context(), actor, subjectattendancepkg.TeachingAssignmentFilter{
		TeacherID: strings.TrimSpace(c.Query("teacher_id")),
		SubjectID: strings.TrimSpace(c.Query("subject_id")),
		ClassID:   strings.TrimSpace(c.Query("class_id")),
		IsActive:  isActive,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

// GetAssignment handles GET /teaching-assignments/:id -> { "assignment": {...} }.
func (h *subjectAttendanceHandler) GetAssignment(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	assignment, err := h.service.GetTeachingAssignment(c.Request.Context(), actor, c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"assignment": assignment})
}

// CreateAssignment handles POST /teaching-assignments -> 201 { "assignment": {...} }.
func (h *subjectAttendanceHandler) CreateAssignment(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request assignmentRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, subjectattendancepkg.ErrInvalidInput)
		return
	}
	assignment, err := h.service.CreateTeachingAssignment(c.Request.Context(), actor, subjectattendancepkg.TeachingAssignmentInput{
		TeacherID: request.TeacherID,
		SubjectID: request.SubjectID,
		ClassID:   request.ClassID,
		IsActive:  request.IsActive,
	}, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, gin.H{"assignment": assignment})
}

// UpdateAssignment handles PUT /teaching-assignments/:id -> { "assignment": {...} }.
func (h *subjectAttendanceHandler) UpdateAssignment(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request assignmentRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, subjectattendancepkg.ErrInvalidInput)
		return
	}
	assignment, err := h.service.UpdateTeachingAssignment(c.Request.Context(), actor, c.Param("id"), subjectattendancepkg.TeachingAssignmentInput{
		TeacherID: request.TeacherID,
		SubjectID: request.SubjectID,
		ClassID:   request.ClassID,
		IsActive:  request.IsActive,
	}, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"assignment": assignment})
}

// CreateSession handles POST /attendance-sessions -> 201 { "session": {...} }.
func (h *subjectAttendanceHandler) CreateSession(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request sessionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, subjectattendancepkg.ErrInvalidInput)
		return
	}
	session, err := h.service.CreateSession(c.Request.Context(), actor, subjectattendancepkg.SessionInput{
		TeachingAssignmentID: request.TeachingAssignmentID,
		SessionDate:          request.SessionDate,
		PeriodStart:          request.PeriodStart,
		PeriodEnd:            request.PeriodEnd,
		Topic:                request.Topic,
		Notes:                request.Notes,
	}, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, gin.H{"session": session})
}

// ListSessions handles GET /attendance-sessions?from=&to=&class_id=&subject_id=&teacher_id=&status=&limit=&offset=.
func (h *subjectAttendanceHandler) ListSessions(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	limit, err := int32Query(c.Query("limit"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	offset, err := int32Query(c.Query("offset"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	result, err := h.service.ListSessions(c.Request.Context(), actor, subjectattendancepkg.SessionFilter{
		From:      strings.TrimSpace(c.Query("from")),
		To:        strings.TrimSpace(c.Query("to")),
		ClassID:   strings.TrimSpace(c.Query("class_id")),
		SubjectID: strings.TrimSpace(c.Query("subject_id")),
		TeacherID: strings.TrimSpace(c.Query("teacher_id")),
		Status:    strings.TrimSpace(c.Query("status")),
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

// GetSession handles GET /attendance-sessions/:id -> { "session": {...} }.
func (h *subjectAttendanceHandler) GetSession(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	session, err := h.service.GetSession(c.Request.Context(), actor, c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"session": session})
}

// Roster handles GET /attendance-sessions/:id/roster -> { "data": [...] }.
func (h *subjectAttendanceHandler) Roster(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	result, err := h.service.Roster(c.Request.Context(), actor, c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

// Records handles GET /attendance-sessions/:id/records -> { "data": [...] }.
func (h *subjectAttendanceHandler) Records(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	result, err := h.service.Records(c.Request.Context(), actor, c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

// SaveRecords handles PUT /attendance-sessions/:id/records -> { "data": [...] }.
func (h *subjectAttendanceHandler) SaveRecords(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request saveRecordsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, subjectattendancepkg.ErrInvalidInput)
		return
	}
	records := make([]subjectattendancepkg.RecordInput, 0, len(request.Records))
	for _, record := range request.Records {
		records = append(records, subjectattendancepkg.RecordInput{
			StudentID: record.StudentID,
			Status:    record.Status,
			Note:      record.Note,
		})
	}
	result, err := h.service.SaveRecords(c.Request.Context(), actor, c.Param("id"),
		subjectattendancepkg.SaveRecordsInput{Records: records}, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

// CloseSession handles POST /attendance-sessions/:id/close -> { "session": {...} }.
func (h *subjectAttendanceHandler) CloseSession(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	session, err := h.service.CloseSession(c.Request.Context(), actor, c.Param("id"), response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"session": session})
}

// Summary handles GET /reports/subject-attendance?from=&to=&class_id=&subject_id=&teacher_id=.
func (h *subjectAttendanceHandler) Summary(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	result, err := h.service.Summary(c.Request.Context(), actor, subjectattendancepkg.SummaryFilter{
		From:      strings.TrimSpace(c.Query("from")),
		To:        strings.TrimSpace(c.Query("to")),
		ClassID:   strings.TrimSpace(c.Query("class_id")),
		SubjectID: strings.TrimSpace(c.Query("subject_id")),
		TeacherID: strings.TrimSpace(c.Query("teacher_id")),
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

func (h *subjectAttendanceHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, subjectattendancepkg.ErrUnauthorized):
		c.Header("WWW-Authenticate", "Bearer")
		response.Error(c, http.StatusUnauthorized, "unauthorized", "authentication required")
	case errors.Is(err, subjectattendancepkg.ErrForbidden):
		response.Error(c, http.StatusForbidden, "forbidden", "insufficient permissions")
	case errors.Is(err, subjectattendancepkg.ErrInvalidInput):
		response.Error(c, http.StatusBadRequest, "invalid_request", "invalid request")
	case errors.Is(err, subjectattendancepkg.ErrNotFound):
		response.Error(c, http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, subjectattendancepkg.ErrConflict):
		response.Error(c, http.StatusConflict, "conflict", "resource already exists or session is closed")
	default:
		h.log.Error("subject_attendance_error", "request_id", response.RequestID(c), "error", err)
		response.Error(c, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func boolQuery(raw string) (*bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	value, err := strconv.ParseBool(trimmed)
	if err != nil {
		return nil, subjectattendancepkg.ErrInvalidInput
	}
	return &value, nil
}

func int32Query(raw string) (int32, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, subjectattendancepkg.ErrInvalidInput
	}
	return int32(value), nil
}
