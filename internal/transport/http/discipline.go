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

func parsePositiveID(c *gin.Context, paramName string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(paramName), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, http.StatusBadRequest, "invalid_id", paramName+" must be a positive integer")
		return 0, false
	}
	return id, true
}

func (h *disciplineHandler) ListViolationTypes(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	includeInactive := c.Query("include_inactive") == "true"
	types, err := h.service.ListViolationTypes(c.Request.Context(), actor, includeInactive)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, types)
}

type upsertViolationTypeRequest struct {
	Kode        string `json:"kode" binding:"required"`
	Nama        string `json:"nama" binding:"required"`
	Kategori    string `json:"kategori" binding:"required"`
	PoinDefault int32  `json:"poin_default"`
	IsActive    *bool  `json:"is_active"`
}

func (h *disciplineHandler) CreateViolationType(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var req upsertViolationTypeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "invalid_request", "kode, nama, and kategori are required")
		return
	}
	created, err := h.service.CreateViolationType(c.Request.Context(), actor, discipline.UpsertViolationTypeInput{
		Kode:        req.Kode,
		Nama:        req.Nama,
		Kategori:    req.Kategori,
		PoinDefault: req.PoinDefault,
		IsActive:    req.IsActive,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, created)
}

func (h *disciplineHandler) UpdateViolationType(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	id, ok := parsePositiveID(c, "id")
	if !ok {
		return
	}
	var req upsertViolationTypeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "invalid_request", "kode, nama, and kategori are required")
		return
	}
	updated, err := h.service.UpdateViolationType(c.Request.Context(), actor, id, discipline.UpsertViolationTypeInput{
		Kode:        req.Kode,
		Nama:        req.Nama,
		Kategori:    req.Kategori,
		PoinDefault: req.PoinDefault,
		IsActive:    req.IsActive,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, updated)
}

func (h *disciplineHandler) DeleteViolationType(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	id, ok := parsePositiveID(c, "id")
	if !ok {
		return
	}
	if err := h.service.DeleteViolationType(c.Request.Context(), actor, id); err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"deleted": true})
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

type updateViolationRequest struct {
	ViolationTypeID int64   `json:"violation_type_id" binding:"required"`
	Poin            *int32  `json:"poin"`
	Keterangan      *string `json:"keterangan"`
	TindakLanjut    *string `json:"tindak_lanjut"`
	Tanggal         string  `json:"tanggal" binding:"required"`
}

func (h *disciplineHandler) UpdateViolation(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	id, ok := parsePositiveID(c, "id")
	if !ok {
		return
	}
	var req updateViolationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "invalid_request", "violation_type_id and tanggal (YYYY-MM-DD) are required")
		return
	}
	updated, err := h.service.UpdateViolation(c.Request.Context(), actor, id, discipline.UpdateViolationInput{
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
	response.JSON(c, http.StatusOK, updated)
}

func (h *disciplineHandler) DeleteViolation(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	id, ok := parsePositiveID(c, "id")
	if !ok {
		return
	}
	if err := h.service.DeleteViolation(c.Request.Context(), actor, id); err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"deleted": true})
}

func (h *disciplineHandler) GetStudentSummary(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	studentID, ok := parsePositiveID(c, "id")
	if !ok {
		return
	}

	summary, err := h.service.GetStudentSummary(c.Request.Context(), actor, studentID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, summary)
}

func (h *disciplineHandler) ListSchoolStudents(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var classID *int64
	if val := c.Query("class_id"); val != "" {
		if id, err := strconv.ParseInt(val, 10, 64); err == nil && id > 0 {
			classID = &id
		}
	}
	students, err := h.service.ListSchoolStudents(c.Request.Context(), actor, classID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, students)
}

func (h *disciplineHandler) ListWatchlist(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var minPoints int64 = 1
	if val := c.Query("min_points"); val != "" {
		if parsed, err := strconv.ParseInt(val, 10, 64); err == nil && parsed >= 0 {
			minPoints = parsed
		}
	}
	watchlist, err := h.service.ListWatchlist(c.Request.Context(), actor, minPoints)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, watchlist)
}

type updateFollowupRequest struct {
	TindakLanjut string `json:"tindak_lanjut" binding:"required"`
}

func (h *disciplineHandler) UpdateFollowup(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	id, ok := parsePositiveID(c, "id")
	if !ok {
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
	ClassID         int64                                      `json:"class_id" binding:"required"`
	Tanggal         string                                     `json:"tanggal" binding:"required"`
	JumlahDiperiksa int32                                      `json:"jumlah_diperiksa" binding:"required"`
	Catatan         *string                                    `json:"catatan"`
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

type updateInspectionRequest struct {
	Tanggal         string                                     `json:"tanggal" binding:"required"`
	JumlahDiperiksa int32                                      `json:"jumlah_diperiksa"`
	Catatan         *string                                    `json:"catatan"`
	Violations      []discipline.CreateInspectionViolationItem `json:"violations"`
}

func (h *disciplineHandler) UpdateInspection(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	id, ok := parsePositiveID(c, "id")
	if !ok {
		return
	}
	var req updateInspectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "invalid_request", "tanggal is required")
		return
	}
	updated, err := h.service.UpdateInspection(c.Request.Context(), actor, id, discipline.UpdateInspectionInput{
		Tanggal:         req.Tanggal,
		JumlahDiperiksa: req.JumlahDiperiksa,
		Catatan:         req.Catatan,
		Violations:      req.Violations,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, updated)
}

func (h *disciplineHandler) DeleteInspection(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	id, ok := parsePositiveID(c, "id")
	if !ok {
		return
	}
	if err := h.service.DeleteInspection(c.Request.Context(), actor, id); err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"deleted": true})
}

func (h *disciplineHandler) ListClassStudents(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	classID, ok := parsePositiveID(c, "id")
	if !ok {
		return
	}

	students, err := h.service.ListClassStudents(c.Request.Context(), actor, classID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, students)
}

func (h *disciplineHandler) ListCounselingRecords(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	filter := discipline.ListCounselingFilter{}
	if val := c.Query("student_id"); val != "" {
		if id, err := strconv.ParseInt(val, 10, 64); err == nil && id > 0 {
			filter.StudentID = &id
		}
	}
	if st := c.Query("status"); st != "" {
		filter.Status = &st
	}
	items, err := h.service.ListCounselingRecords(c.Request.Context(), actor, filter)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, items)
}

type createCounselingRequest struct {
	StudentID    int64   `json:"student_id" binding:"required"`
	Tanggal      string  `json:"tanggal" binding:"required"`
	Topik        string  `json:"topik" binding:"required"`
	Catatan      *string `json:"catatan"`
	TindakLanjut *string `json:"tindak_lanjut"`
	Status       string  `json:"status"`
}

func (h *disciplineHandler) CreateCounselingRecord(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var req createCounselingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "invalid_request", "student_id, tanggal, and topik are required")
		return
	}
	created, err := h.service.CreateCounselingRecord(c.Request.Context(), actor, discipline.CreateCounselingInput{
		StudentID:    req.StudentID,
		Tanggal:      req.Tanggal,
		Topik:        req.Topik,
		Catatan:      req.Catatan,
		TindakLanjut: req.TindakLanjut,
		Status:       req.Status,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, created)
}

type updateCounselingRequest struct {
	Tanggal      string  `json:"tanggal" binding:"required"`
	Topik        string  `json:"topik" binding:"required"`
	Catatan      *string `json:"catatan"`
	TindakLanjut *string `json:"tindak_lanjut"`
	Status       string  `json:"status" binding:"required"`
}

func (h *disciplineHandler) UpdateCounselingRecord(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	id, ok := parsePositiveID(c, "id")
	if !ok {
		return
	}
	var req updateCounselingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "invalid_request", "tanggal, topik, and status are required")
		return
	}
	updated, err := h.service.UpdateCounselingRecord(c.Request.Context(), actor, id, discipline.UpdateCounselingInput{
		Tanggal:      req.Tanggal,
		Topik:        req.Topik,
		Catatan:      req.Catatan,
		TindakLanjut: req.TindakLanjut,
		Status:       req.Status,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, updated)
}

func (h *disciplineHandler) DeleteCounselingRecord(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	id, ok := parsePositiveID(c, "id")
	if !ok {
		return
	}
	if err := h.service.DeleteCounselingRecord(c.Request.Context(), actor, id); err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"deleted": true})
}

func (h *disciplineHandler) ListParentSummons(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	filter := discipline.ListSummonsFilter{}
	if val := c.Query("student_id"); val != "" {
		if id, err := strconv.ParseInt(val, 10, 64); err == nil && id > 0 {
			filter.StudentID = &id
		}
	}
	if st := c.Query("status"); st != "" {
		filter.Status = &st
	}
	items, err := h.service.ListParentSummons(c.Request.Context(), actor, filter)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, items)
}

type createParentSummonsRequest struct {
	StudentID       int64   `json:"student_id" binding:"required"`
	NomorSurat      string  `json:"nomor_surat" binding:"required"`
	TanggalSurat    string  `json:"tanggal_surat" binding:"required"`
	JadwalPertemuan string  `json:"jadwal_pertemuan" binding:"required"`
	Ruangan         string  `json:"ruangan" binding:"required"`
	Alasan          string  `json:"alasan" binding:"required"`
	NamaOrtu        *string `json:"nama_ortu"`
	NoHpOrtu        *string `json:"no_hp_ortu"`
	Status          string  `json:"status"`
}

func (h *disciplineHandler) CreateParentSummons(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var req createParentSummonsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "invalid_request", "student_id, nomor_surat, tanggal_surat, jadwal_pertemuan, ruangan, and alasan are required")
		return
	}
	created, err := h.service.CreateParentSummons(c.Request.Context(), actor, discipline.CreateParentSummonsInput{
		StudentID:       req.StudentID,
		NomorSurat:      req.NomorSurat,
		TanggalSurat:    req.TanggalSurat,
		JadwalPertemuan: req.JadwalPertemuan,
		Ruangan:         req.Ruangan,
		Alasan:          req.Alasan,
		NamaOrtu:        req.NamaOrtu,
		NoHpOrtu:        req.NoHpOrtu,
		Status:          req.Status,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, created)
}

type updateParentSummonsRequest struct {
	NomorSurat      string  `json:"nomor_surat" binding:"required"`
	TanggalSurat    string  `json:"tanggal_surat" binding:"required"`
	JadwalPertemuan string  `json:"jadwal_pertemuan" binding:"required"`
	Ruangan         string  `json:"ruangan" binding:"required"`
	Alasan          string  `json:"alasan" binding:"required"`
	NamaOrtu        *string `json:"nama_ortu"`
	NoHpOrtu        *string `json:"no_hp_ortu"`
	CatatanHasil    *string `json:"catatan_hasil"`
}

func (h *disciplineHandler) UpdateParentSummons(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	id, ok := parsePositiveID(c, "id")
	if !ok {
		return
	}
	var req updateParentSummonsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "invalid_request", "nomor_surat, tanggal_surat, jadwal_pertemuan, ruangan, and alasan are required")
		return
	}
	updated, err := h.service.UpdateParentSummons(c.Request.Context(), actor, id, discipline.UpdateParentSummonsInput{
		NomorSurat:      req.NomorSurat,
		TanggalSurat:    req.TanggalSurat,
		JadwalPertemuan: req.JadwalPertemuan,
		Ruangan:         req.Ruangan,
		Alasan:          req.Alasan,
		NamaOrtu:        req.NamaOrtu,
		NoHpOrtu:        req.NoHpOrtu,
		CatatanHasil:    req.CatatanHasil,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, updated)
}

type updateParentSummonsStatusRequest struct {
	Status       string  `json:"status" binding:"required"`
	CatatanHasil *string `json:"catatan_hasil"`
}

func (h *disciplineHandler) UpdateParentSummonsStatus(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	id, ok := parsePositiveID(c, "id")
	if !ok {
		return
	}
	var req updateParentSummonsStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "invalid_request", "status is required")
		return
	}
	updated, err := h.service.UpdateParentSummonsStatus(c.Request.Context(), actor, id, req.Status, req.CatatanHasil)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, updated)
}

func (h *disciplineHandler) DeleteParentSummons(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	id, ok := parsePositiveID(c, "id")
	if !ok {
		return
	}
	if err := h.service.DeleteParentSummons(c.Request.Context(), actor, id); err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"deleted": true})
}

func (h *disciplineHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, discipline.ErrNotFound):
		response.Error(c, http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, discipline.ErrConflict):
		response.Error(c, http.StatusConflict, "conflict", "data already exists or is referenced by other records")
	case errors.Is(err, discipline.ErrForbidden):
		response.Error(c, http.StatusForbidden, "forbidden", "access forbidden")
	case errors.Is(err, discipline.ErrNotStaff):
		response.Error(c, http.StatusForbidden, "not_staff", "token subject is not an active staff account")
	case errors.Is(err, discipline.ErrInvalidStudent):
		response.Error(c, http.StatusUnprocessableEntity, "invalid_student", "student_id must belong to your school")
	case errors.Is(err, discipline.ErrInvalidViolationType):
		response.Error(c, http.StatusUnprocessableEntity, "invalid_violation_type", "violation_type_id must be valid")
	case errors.Is(err, discipline.ErrInvalidCategory):
		response.Error(c, http.StatusUnprocessableEntity, "invalid_category", "kategori must be atribut, kerapihan, perilaku, or lainnya")
	case errors.Is(err, discipline.ErrInvalidStatus):
		response.Error(c, http.StatusUnprocessableEntity, "invalid_status", "invalid status value")
	case errors.Is(err, discipline.ErrInvalidClass):
		response.Error(c, http.StatusUnprocessableEntity, "invalid_class", "class_id must belong to your school")
	case errors.Is(err, discipline.ErrInvalidDate):
		response.Error(c, http.StatusUnprocessableEntity, "invalid_date", "tanggal must be YYYY-MM-DD")
	case errors.Is(err, discipline.ErrInvalidDateTime):
		response.Error(c, http.StatusUnprocessableEntity, "invalid_datetime", "jadwal_pertemuan must be RFC3339 or YYYY-MM-DDTHH:MM")
	case errors.Is(err, discipline.ErrInvalidPoints):
		response.Error(c, http.StatusUnprocessableEntity, "invalid_points", "points must be non-negative")
	case errors.Is(err, discipline.ErrInvalidPayload):
		response.Error(c, http.StatusUnprocessableEntity, "invalid_payload", "required fields must not be empty")
	default:
		h.log.Error("discipline_request_failed", "request_id", response.RequestID(c), "error", err)
		response.Error(c, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
