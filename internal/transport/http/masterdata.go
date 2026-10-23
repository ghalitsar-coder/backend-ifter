package httptransport

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/masterdata"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

type masterDataHandler struct {
	service *masterdata.Service
	log     *slog.Logger
}

type academicYearRequest struct {
	Name      string `json:"name" binding:"required"`
	StartDate string `json:"start_date" binding:"required"`
	EndDate   string `json:"end_date" binding:"required"`
	IsActive  bool   `json:"is_active"`
}

type termRequest struct {
	TermNumber int16  `json:"term_number" binding:"required"`
	Name       string `json:"name" binding:"required"`
	StartDate  string `json:"start_date" binding:"required"`
	EndDate    string `json:"end_date" binding:"required"`
}

type classRequest struct {
	AcademicYearID string `json:"academic_year_id" binding:"required"`
	Name           string `json:"name" binding:"required"`
	GradeLevel     int16  `json:"grade_level" binding:"required"`
	Major          string `json:"major"`
}

type classUpdateRequest struct {
	Name       string `json:"name" binding:"required"`
	GradeLevel int16  `json:"grade_level" binding:"required"`
	Major      string `json:"major"`
}

type classStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

type homeroomRequest struct {
	TeacherID string `json:"teacher_id" binding:"required"`
	StartDate string `json:"start_date"`
}

type teacherRequest struct {
	EmployeeNumber string `json:"employee_number"`
	FullName       string `json:"full_name" binding:"required"`
	Email          string `json:"email"`
	Phone          string `json:"phone"`
	UserID         string `json:"user_id"`
	Status         string `json:"status"`
}

type teacherUpdateRequest struct {
	EmployeeNumber string `json:"employee_number"`
	FullName       string `json:"full_name" binding:"required"`
	Email          string `json:"email"`
	Phone          string `json:"phone"`
	UserID         string `json:"user_id"`
}

type teacherStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

type studentRequest struct {
	StudentNumber         string `json:"student_number" binding:"required"`
	NationalStudentNumber string `json:"national_student_number"`
	FullName              string `json:"full_name" binding:"required"`
	Gender                string `json:"gender"`
	BirthDate             string `json:"birth_date"`
	AdmissionYear         int16  `json:"admission_year"`
	Status                string `json:"status"`
	GuardianName          string `json:"guardian_name"`
	GuardianPhone         string `json:"guardian_phone"`
	ClassID               string `json:"class_id"`
	ClassStartDate        string `json:"class_start_date"`
}

type studentUpdateRequest struct {
	StudentNumber         string `json:"student_number" binding:"required"`
	NationalStudentNumber string `json:"national_student_number"`
	FullName              string `json:"full_name" binding:"required"`
	Gender                string `json:"gender"`
	BirthDate             string `json:"birth_date"`
	AdmissionYear         int16  `json:"admission_year"`
	GuardianName          string `json:"guardian_name"`
	GuardianPhone         string `json:"guardian_phone"`
}

type studentStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

type studentClassRequest struct {
	ClassID   string `json:"class_id" binding:"required"`
	StartDate string `json:"start_date"`
}

func newMasterDataHandler(service *masterdata.Service, log *slog.Logger) *masterDataHandler {
	return &masterDataHandler{service: service, log: log}
}

func (h *masterDataHandler) ListAcademicYears(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	years, err := h.service.ListAcademicYears(c.Request.Context())
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": years})
}

func (h *masterDataHandler) GetAcademicYear(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	year, err := h.service.GetAcademicYear(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"academic_year": year})
}

func (h *masterDataHandler) ListTerms(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	terms, err := h.service.ListTerms(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": terms})
}

func (h *masterDataHandler) GetTerm(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	term, err := h.service.GetTerm(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"term": term})
}

func (h *masterDataHandler) CreateTerm(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req termRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_request", "term_number, name, start_date, and end_date are required")
		return
	}
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	term, err := h.service.CreateTerm(c.Request.Context(), actor, c.Param("id"), masterdata.TermInput{
		TermNumber: req.TermNumber,
		Name:       req.Name,
		StartDate:  req.StartDate,
		EndDate:    req.EndDate,
	}, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"term": term})
}

func (h *masterDataHandler) UpdateTerm(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req termRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_request", "term_number, name, start_date, and end_date are required")
		return
	}
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	term, err := h.service.UpdateTerm(c.Request.Context(), actor, c.Param("id"), masterdata.TermInput{
		TermNumber: req.TermNumber,
		Name:       req.Name,
		StartDate:  req.StartDate,
		EndDate:    req.EndDate,
	}, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"term": term})
}

func (h *masterDataHandler) CreateAcademicYear(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req academicYearRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_request", "name, start_date, and end_date are required")
		return
	}
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	year, err := h.service.CreateAcademicYear(c.Request.Context(), actor, masterdata.AcademicYearInput{
		Name:      req.Name,
		StartDate: req.StartDate,
		EndDate:   req.EndDate,
		IsActive:  req.IsActive,
	}, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"academic_year": year})
}

func (h *masterDataHandler) UpdateAcademicYear(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req academicYearRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_request", "name, start_date, and end_date are required")
		return
	}
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	year, err := h.service.UpdateAcademicYear(c.Request.Context(), actor, c.Param("id"), masterdata.AcademicYearInput{
		Name:      req.Name,
		StartDate: req.StartDate,
		EndDate:   req.EndDate,
		IsActive:  req.IsActive,
	}, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"academic_year": year})
}

func (h *masterDataHandler) ListClasses(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	page, err := parsePage(c)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_request", "invalid pagination")
		return
	}
	classes, total, err := h.service.ListClasses(c.Request.Context(), masterdata.ClassFilter{
		AcademicYearID: c.Query("academic_year_id"),
		Status:         c.Query("status"),
	}, page)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": classes, "total": total, "limit": page.Limit, "offset": page.Offset})
}

func (h *masterDataHandler) GetClass(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	class, err := h.service.GetClass(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"class": class})
}

func (h *masterDataHandler) CreateClass(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req classRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_request", "academic_year_id, name, and grade_level are required")
		return
	}
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	class, err := h.service.CreateClass(c.Request.Context(), actor, masterdata.ClassInput{
		AcademicYearID: req.AcademicYearID,
		Name:           req.Name,
		GradeLevel:     req.GradeLevel,
		Major:          req.Major,
	}, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"class": class})
}

func (h *masterDataHandler) UpdateClass(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req classUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_request", "name and grade_level are required")
		return
	}
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	class, err := h.service.UpdateClass(c.Request.Context(), actor, c.Param("id"), masterdata.ClassUpdateInput{
		Name:       req.Name,
		GradeLevel: req.GradeLevel,
		Major:      req.Major,
	}, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"class": class})
}

func (h *masterDataHandler) UpdateClassStatus(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req classStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_request", "status is required")
		return
	}
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	class, err := h.service.UpdateClassStatus(c.Request.Context(), actor, c.Param("id"), req.Status, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"class": class})
}

func (h *masterDataHandler) ListClassStudents(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	students, err := h.service.ListClassStudents(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": students})
}

func (h *masterDataHandler) AssignHomeroom(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req homeroomRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_request", "teacher_id is required")
		return
	}
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	assignment, err := h.service.AssignHomeroom(c.Request.Context(), actor, c.Param("id"), masterdata.HomeroomInput{
		TeacherID: req.TeacherID,
		StartDate: req.StartDate,
	}, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"homeroom": assignment})
}

func (h *masterDataHandler) ListTeachers(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	page, err := parsePage(c)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_request", "invalid pagination")
		return
	}
	teachers, total, err := h.service.ListTeachers(c.Request.Context(), masterdata.TeacherFilter{
		Search: c.Query("search"),
		Status: c.Query("status"),
	}, page)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": teachers, "total": total, "limit": page.Limit, "offset": page.Offset})
}

func (h *masterDataHandler) GetTeacher(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	teacher, err := h.service.GetTeacher(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"teacher": teacher})
}

func (h *masterDataHandler) CreateTeacher(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req teacherRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_request", "full_name is required")
		return
	}
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	teacher, err := h.service.CreateTeacher(c.Request.Context(), actor, masterdata.TeacherInput{
		EmployeeNumber: req.EmployeeNumber,
		FullName:       req.FullName,
		Email:          req.Email,
		Phone:          req.Phone,
		UserID:         req.UserID,
		Status:         req.Status,
	}, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"teacher": teacher})
}

func (h *masterDataHandler) UpdateTeacher(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req teacherUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_request", "full_name is required")
		return
	}
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	teacher, err := h.service.UpdateTeacher(c.Request.Context(), actor, c.Param("id"), masterdata.TeacherUpdateInput{
		EmployeeNumber: req.EmployeeNumber,
		FullName:       req.FullName,
		Email:          req.Email,
		Phone:          req.Phone,
		UserID:         req.UserID,
	}, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"teacher": teacher})
}

func (h *masterDataHandler) UpdateTeacherStatus(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req teacherStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_request", "status is required")
		return
	}
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	teacher, err := h.service.UpdateTeacherStatus(c.Request.Context(), actor, c.Param("id"), req.Status, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"teacher": teacher})
}

func (h *masterDataHandler) ListStudents(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	page, err := parsePage(c)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_request", "invalid pagination")
		return
	}
	students, total, err := h.service.ListStudents(c.Request.Context(), masterdata.StudentFilter{
		Search:  c.Query("search"),
		ClassID: c.Query("class_id"),
		Status:  c.Query("status"),
		SortBy:  c.Query("sort"),
	}, page)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": students, "total": total, "limit": page.Limit, "offset": page.Offset})
}

func (h *masterDataHandler) GetStudent(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	student, err := h.service.GetStudent(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"student": student})
}

func (h *masterDataHandler) CreateStudent(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req studentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_request", "student_number and full_name are required")
		return
	}
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	student, err := h.service.CreateStudent(c.Request.Context(), actor, masterdata.StudentInput{
		StudentNumber:         req.StudentNumber,
		NationalStudentNumber: req.NationalStudentNumber,
		FullName:              req.FullName,
		Gender:                req.Gender,
		BirthDate:             req.BirthDate,
		AdmissionYear:         req.AdmissionYear,
		Status:                req.Status,
		GuardianName:          req.GuardianName,
		GuardianPhone:         req.GuardianPhone,
		ClassID:               req.ClassID,
		ClassStartDate:        req.ClassStartDate,
	}, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"student": student})
}

func (h *masterDataHandler) UpdateStudent(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req studentUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_request", "student_number and full_name are required")
		return
	}
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	student, err := h.service.UpdateStudent(c.Request.Context(), actor, c.Param("id"), masterdata.StudentUpdateInput{
		StudentNumber:         req.StudentNumber,
		NationalStudentNumber: req.NationalStudentNumber,
		FullName:              req.FullName,
		Gender:                req.Gender,
		BirthDate:             req.BirthDate,
		AdmissionYear:         req.AdmissionYear,
		GuardianName:          req.GuardianName,
		GuardianPhone:         req.GuardianPhone,
	}, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"student": student})
}

func (h *masterDataHandler) UpdateStudentStatus(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req studentStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_request", "status is required")
		return
	}
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	student, err := h.service.UpdateStudentStatus(c.Request.Context(), actor, c.Param("id"), req.Status, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"student": student})
}

func (h *masterDataHandler) AssignStudentClass(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req studentClassRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_request", "class_id is required")
		return
	}
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	student, err := h.service.AssignStudentClass(c.Request.Context(), actor, c.Param("id"), masterdata.StudentClassInput{
		ClassID:   req.ClassID,
		StartDate: req.StartDate,
	}, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"student": student})
}

func (h *masterDataHandler) actor(c *gin.Context) (string, bool) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		h.writeError(c, masterdata.ErrUnauthorized)
		return "", false
	}
	return user.ID, true
}

func (h *masterDataHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, masterdata.ErrUnauthorized):
		c.Header("WWW-Authenticate", "Bearer")
		response.Error(c, http.StatusUnauthorized, "unauthorized", "authentication required")
	case errors.Is(err, masterdata.ErrInvalidInput):
		response.Error(c, http.StatusBadRequest, "invalid_request", "invalid request")
	case errors.Is(err, masterdata.ErrNotFound):
		response.Error(c, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, masterdata.ErrConflict):
		response.Error(c, http.StatusConflict, "conflict", "data conflicts with an existing record")
	default:
		h.log.Error("master_data_error", "request_id", response.RequestID(c), "error", err)
		response.Error(c, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func parsePage(c *gin.Context) (masterdata.Page, error) {
	page := masterdata.Page{Limit: 50}
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || value < 1 || value > 100 {
			return page, errors.New("invalid limit")
		}
		page.Limit = int32(value)
	}
	if raw := strings.TrimSpace(c.Query("offset")); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || value < 0 {
			return page, errors.New("invalid offset")
		}
		page.Offset = int32(value)
	}
	return page, nil
}
