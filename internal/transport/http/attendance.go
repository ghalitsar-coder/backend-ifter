package httptransport

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/attendance"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

type attendanceHandler struct {
	service *attendance.Service
	log     *slog.Logger
}

type attendanceRecordRequest struct {
	StudentID string `json:"student_id" binding:"required"`
	Status    string `json:"status" binding:"required"`
	CheckInAt string `json:"check_in_at"`
	Notes     string `json:"notes"`
}

type attendanceSaveRequest struct {
	ClassID string                    `json:"class_id" binding:"required"`
	Date    string                    `json:"date"`
	Records []attendanceRecordRequest `json:"records" binding:"required"`
}

type attendanceCloseRequest struct {
	ClassID string `json:"class_id" binding:"required"`
	Date    string `json:"date"`
}

type attendanceCorrectionRequest struct {
	Status    *string `json:"status"`
	CheckInAt *string `json:"check_in_at"`
	Notes     *string `json:"notes"`
	Reason    string  `json:"reason" binding:"required"`
}

type scheduleRequest struct {
	Weekday              int16  `json:"weekday" binding:"required"`
	StartTime            string `json:"start_time" binding:"required"`
	LateToleranceMinutes int16  `json:"late_tolerance_minutes"`
}

type attendanceSettingsRequest struct {
	SchoolName    string            `json:"school_name" binding:"required"`
	Timezone      string            `json:"timezone" binding:"required"`
	AutoCloseTime string            `json:"attendance_auto_close_time" binding:"required"`
	Schedules     []scheduleRequest `json:"schedules"`
}

type holidayRequest struct {
	Date string `json:"holiday_date" binding:"required"`
	Name string `json:"name" binding:"required"`
}

type dutyAssignmentRequest struct {
	TeacherID string `json:"teacher_id" binding:"required"`
	Weekday   int16  `json:"weekday" binding:"required"`
	StartDate string `json:"start_date" binding:"required"`
	EndDate   string `json:"end_date"`
}

type dutyAssignmentEndRequest struct {
	EndDate string `json:"end_date" binding:"required"`
}

func newAttendanceHandler(service *attendance.Service, log *slog.Logger) *attendanceHandler {
	return &attendanceHandler{service: service, log: log}
}

func (h *attendanceHandler) actor(c *gin.Context) (attendance.Actor, bool) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		h.writeError(c, attendance.ErrUnauthorized)
		return attendance.Actor{}, false
	}
	return attendance.Actor{ID: user.ID, Roles: user.Roles}, true
}

func (h *attendanceHandler) Roster(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	result, err := h.service.GetRoster(c.Request.Context(), actor, c.Query("class_id"), c.Query("date"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

func (h *attendanceHandler) Save(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request attendanceSaveRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, attendance.ErrInvalidInput)
		return
	}
	records := make([]attendance.RecordInput, 0, len(request.Records))
	for _, item := range request.Records {
		records = append(records, attendance.RecordInput{
			StudentID: item.StudentID,
			Status:    item.Status,
			CheckInAt: item.CheckInAt,
			Notes:     item.Notes,
		})
	}
	result, err := h.service.Save(c.Request.Context(), actor, attendance.SaveInput{
		ClassID: request.ClassID,
		Date:    request.Date,
		Records: records,
	}, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

func (h *attendanceHandler) Close(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request attendanceCloseRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, attendance.ErrInvalidInput)
		return
	}
	result, err := h.service.Close(c.Request.Context(), actor, request.ClassID, request.Date, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

func (h *attendanceHandler) GetRecord(c *gin.Context) {
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

func (h *attendanceHandler) Correct(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request attendanceCorrectionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, attendance.ErrInvalidInput)
		return
	}
	result, err := h.service.Correct(c.Request.Context(), actor, c.Param("id"), attendance.CorrectionInput{
		Status:    request.Status,
		CheckInAt: request.CheckInAt,
		Notes:     request.Notes,
		Reason:    request.Reason,
	}, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

func (h *attendanceHandler) History(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	limit, offset, err := parseAttendancePage(c)
	if err != nil {
		h.writeError(c, attendance.ErrInvalidInput)
		return
	}
	result, err := h.service.History(c.Request.Context(), actor, attendance.HistoryFilter{
		From:      c.Query("from"),
		To:        c.Query("to"),
		StudentID: c.Query("student_id"),
		ClassID:   c.Query("class_id"),
		Status:    c.Query("status"),
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

func (h *attendanceHandler) Summary(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	result, err := h.service.Summary(c.Request.Context(), actor, attendance.SummaryFilter{
		From:      c.Query("from"),
		To:        c.Query("to"),
		StudentID: c.Query("student_id"),
		ClassID:   c.Query("class_id"),
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

func (h *attendanceHandler) Export(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	content, err := h.service.Export(c.Request.Context(), actor, attendance.SummaryFilter{
		From:      c.Query("from"),
		To:        c.Query("to"),
		StudentID: c.Query("student_id"),
		ClassID:   c.Query("class_id"),
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	filename := "rekap-absensi-" + c.Query("from") + "-" + c.Query("to") + ".xlsx"
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", content)
}

func (h *attendanceHandler) Report(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	result, err := h.service.Report(c.Request.Context(), actor, attendance.ReportFilter{
		From:      c.Query("from"),
		To:        c.Query("to"),
		TermID:    c.Query("term_id"),
		GroupBy:   c.Query("group_by"),
		StudentID: c.Query("student_id"),
		ClassID:   c.Query("class_id"),
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

func (h *attendanceHandler) ExportReport(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	content, err := h.service.ExportReport(c.Request.Context(), actor, attendance.ReportFilter{
		From:      c.Query("from"),
		To:        c.Query("to"),
		TermID:    c.Query("term_id"),
		GroupBy:   c.Query("group_by"),
		StudentID: c.Query("student_id"),
		ClassID:   c.Query("class_id"),
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	filename := "laporan-absensi-" + c.Query("from") + "-" + c.Query("to") + ".xlsx"
	if termID := c.Query("term_id"); termID != "" {
		filename = "laporan-absensi-term-" + termID + ".xlsx"
	}
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", content)
}

func (h *attendanceHandler) GetSettings(c *gin.Context) {
	result, err := h.service.GetSettings(c.Request.Context())
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

func (h *attendanceHandler) UpdateSettings(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request attendanceSettingsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, attendance.ErrInvalidInput)
		return
	}
	schedules := make([]attendance.ScheduleInput, 0, len(request.Schedules))
	for _, item := range request.Schedules {
		schedules = append(schedules, attendance.ScheduleInput{
			Weekday:              item.Weekday,
			StartTime:            item.StartTime,
			LateToleranceMinutes: item.LateToleranceMinutes,
		})
	}
	result, err := h.service.UpdateSettings(c.Request.Context(), actor, attendance.SettingsInput{
		SchoolName:    request.SchoolName,
		Timezone:      request.Timezone,
		AutoCloseTime: request.AutoCloseTime,
		Schedules:     schedules,
	}, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

func (h *attendanceHandler) ListHolidays(c *gin.Context) {
	result, err := h.service.ListHolidays(c.Request.Context(), c.Query("from"), c.Query("to"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"data": result})
}

func (h *attendanceHandler) CreateHoliday(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request holidayRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, attendance.ErrInvalidInput)
		return
	}
	result, err := h.service.CreateHoliday(c.Request.Context(), actor, attendance.HolidayInput{
		Date: request.Date,
		Name: request.Name,
	}, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, result)
}

func (h *attendanceHandler) ListDutyAssignments(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	weekday := int16(0)
	if raw := strings.TrimSpace(c.Query("weekday")); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 16)
		if err != nil || value < 1 || value > 7 {
			h.writeError(c, attendance.ErrInvalidInput)
			return
		}
		weekday = int16(value)
	}
	result, err := h.service.ListDutyAssignments(c.Request.Context(), actor, attendance.DutyAssignmentFilter{
		TeacherID: c.Query("teacher_id"),
		Weekday:   weekday,
		Date:      c.Query("date"),
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"data": result})
}

func (h *attendanceHandler) CreateDutyAssignment(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request dutyAssignmentRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, attendance.ErrInvalidInput)
		return
	}
	result, err := h.service.CreateDutyAssignment(c.Request.Context(), actor, attendance.DutyAssignmentInput{
		TeacherID: request.TeacherID,
		Weekday:   request.Weekday,
		StartDate: request.StartDate,
		EndDate:   request.EndDate,
	}, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, result)
}

func (h *attendanceHandler) EndDutyAssignment(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request dutyAssignmentEndRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, attendance.ErrInvalidInput)
		return
	}
	result, err := h.service.EndDutyAssignment(c.Request.Context(), actor, c.Param("id"), request.EndDate, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

func (h *attendanceHandler) DeleteHoliday(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	if err := h.service.DeleteHoliday(c.Request.Context(), actor, c.Param("date"), response.RequestID(c)); err != nil {
		h.writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *attendanceHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, attendance.ErrUnauthorized):
		c.Header("WWW-Authenticate", "Bearer")
		response.Error(c, http.StatusUnauthorized, "unauthorized", "authentication required")
	case errors.Is(err, attendance.ErrForbidden):
		response.Error(c, http.StatusForbidden, "forbidden", "insufficient permissions")
	case errors.Is(err, attendance.ErrInvalidInput):
		response.Error(c, http.StatusBadRequest, "invalid_request", "invalid request")
	case errors.Is(err, attendance.ErrNotFound):
		response.Error(c, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, attendance.ErrConflict):
		response.Error(c, http.StatusConflict, "conflict", "attendance data conflicts with an existing record")
	default:
		h.log.Error("attendance_error", "request_id", response.RequestID(c), "error", err)
		response.Error(c, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func parseAttendancePage(c *gin.Context) (int32, int32, error) {
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
