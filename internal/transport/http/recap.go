package httptransport

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
	"github.com/ghalitsar-coder/backend-ifter/internal/recap"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

type recapHandler struct {
	service *recap.Service
	log     *slog.Logger
}

func newRecapHandler(service *recap.Service, log *slog.Logger) *recapHandler {
	return &recapHandler{service: service, log: log}
}

type recapPeriodRequest struct {
	Month int `json:"bulan" binding:"required"`
	Year  int `json:"tahun" binding:"required"`
}

func (h *recapHandler) GenerateStudentRecaps(c *gin.Context) {
	_, schoolID, ok := recapActor(c)
	if !ok {
		return
	}
	var req recapPeriodRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "invalid_request", "bulan and tahun are required")
		return
	}
	items, err := h.service.GenerateStudentRecaps(c.Request.Context(), schoolID, req.Month, req.Year)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, items)
}

func (h *recapHandler) ListStudentRecaps(c *gin.Context) {
	_, schoolID, ok := recapActor(c)
	if !ok {
		return
	}
	month, monthErr := strconv.Atoi(c.Query("bulan"))
	year, yearErr := strconv.Atoi(c.Query("tahun"))
	if monthErr != nil || yearErr != nil {
		response.Error(c, http.StatusUnprocessableEntity, "invalid_period", "bulan and tahun are required integers")
		return
	}
	items, err := h.service.ListStudentRecaps(c.Request.Context(), schoolID, month, year)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, items)
}

func (h *recapHandler) VerifyStudentRecap(c *gin.Context) {
	userID, schoolID, ok := recapActor(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, http.StatusBadRequest, "invalid_id", "id must be a positive integer")
		return
	}
	verified, err := h.service.VerifyStudentRecap(c.Request.Context(), id, schoolID, userID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, verified)
}

func (h *recapHandler) GenerateStaffRecaps(c *gin.Context) {
	_, schoolID, ok := recapActor(c)
	if !ok {
		return
	}
	var req recapPeriodRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "invalid_request", "bulan and tahun are required")
		return
	}
	items, err := h.service.GenerateStaffRecaps(c.Request.Context(), schoolID, req.Month, req.Year)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, items)
}

func (h *recapHandler) ListStaffRecaps(c *gin.Context) {
	_, schoolID, ok := recapActor(c)
	if !ok {
		return
	}
	month, monthErr := strconv.Atoi(c.Query("bulan"))
	year, yearErr := strconv.Atoi(c.Query("tahun"))
	if monthErr != nil || yearErr != nil {
		response.Error(c, http.StatusUnprocessableEntity, "invalid_period", "bulan and tahun are required integers")
		return
	}
	items, err := h.service.ListStaffRecaps(c.Request.Context(), schoolID, month, year)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, items)
}

func (h *recapHandler) VerifyStaffRecap(c *gin.Context) {
	userID, schoolID, ok := recapActor(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, http.StatusBadRequest, "invalid_id", "id must be a positive integer")
		return
	}
	verified, err := h.service.VerifyStaffRecap(c.Request.Context(), id, schoolID, userID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, verified)
}

func recapActor(c *gin.Context) (int64, int64, bool) {
	claims, ok := middleware.CurrentClaims(c)
	if !ok {
		response.Error(c, http.StatusUnauthorized, "unauthorized", "authentication required")
		return 0, 0, false
	}
	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || userID <= 0 {
		response.Error(c, http.StatusForbidden, "invalid_subject", "token subject must be a user id")
		return 0, 0, false
	}
	if claims.SchoolID <= 0 {
		response.Error(c, http.StatusForbidden, "invalid_school", "token must carry a school id")
		return 0, 0, false
	}
	return userID, claims.SchoolID, true
}

func (h *recapHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, recap.ErrForbidden):
		response.Error(c, http.StatusForbidden, "forbidden", "school scope is required")
	case errors.Is(err, recap.ErrInvalidPeriod):
		response.Error(c, http.StatusUnprocessableEntity, "invalid_period", "bulan must be 1-12 and tahun must be a valid year")
	case errors.Is(err, recap.ErrInvalidID):
		response.Error(c, http.StatusBadRequest, "invalid_id", "id must be a positive integer")
	case errors.Is(err, recap.ErrNotFound), database.IsNotFound(err):
		response.Error(c, http.StatusNotFound, "not_found", "student attendance recap not found")
	case errors.Is(err, recap.ErrLocked):
		response.Error(c, http.StatusConflict, "locked", "student attendance recap is already locked")
	default:
		h.log.Error("student_recap_request_failed", "request_id", response.RequestID(c), "error", err)
		response.Error(c, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
