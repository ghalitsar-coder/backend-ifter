package httptransport

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
	"github.com/ghalitsar-coder/backend-ifter/internal/qrattendance"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

type qrAttendanceHandler struct {
	service *qrattendance.Service
	log     *slog.Logger
}

func newQRAttendanceHandler(service *qrattendance.Service, log *slog.Logger) *qrAttendanceHandler {
	return &qrAttendanceHandler{service: service, log: log}
}

func (h *qrAttendanceHandler) CreateSession(c *gin.Context) {
	userID, schoolID, ok := qrActor(c)
	if !ok {
		return
	}
	var input qrattendance.CreateSessionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "invalid_request", "tanggal, tipe_sesi, jam_buka, and jam_tutup are required")
		return
	}
	session, err := h.service.CreateSession(c.Request.Context(), qrattendance.SessionCreator{UserID: userID, SchoolID: schoolID}, input)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, session)
}

func (h *qrAttendanceHandler) CurrentCode(c *gin.Context) {
	_, schoolID, ok := qrActor(c)
	if !ok {
		return
	}
	id, ok := qrSessionID(c)
	if !ok {
		return
	}
	code, err := h.service.CurrentCode(c.Request.Context(), id, schoolID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, code)
}

func (h *qrAttendanceHandler) Scan(c *gin.Context) {
	userID, schoolID, ok := qrActor(c)
	if !ok {
		return
	}
	var req struct {
		Payload string `json:"payload" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "invalid_request", "payload is required")
		return
	}
	result, err := h.service.Scan(c.Request.Context(), qrattendance.ScanInput{
		UserID: userID, SchoolID: schoolID, Payload: req.Payload,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

func (h *qrAttendanceHandler) CloseSession(c *gin.Context) {
	_, schoolID, ok := qrActor(c)
	if !ok {
		return
	}
	id, ok := qrSessionID(c)
	if !ok {
		return
	}
	if err := h.service.CloseSession(c.Request.Context(), id, schoolID); err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"id": id, "status": "selesai"})
}

func qrActor(c *gin.Context) (int64, int64, bool) {
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

func qrSessionID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, http.StatusBadRequest, "invalid_id", "id must be a positive integer")
		return 0, false
	}
	return id, true
}

func (h *qrAttendanceHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, qrattendance.ErrForbidden):
		response.Error(c, http.StatusForbidden, "forbidden", "not allowed to record attendance for this school")
	case errors.Is(err, qrattendance.ErrInvalidSession), errors.Is(err, qrattendance.ErrInvalidDate),
		errors.Is(err, qrattendance.ErrInvalidTime), errors.Is(err, qrattendance.ErrInvalidRotation):
		response.Error(c, http.StatusUnprocessableEntity, "invalid_session", "QR session parameters are invalid")
	case errors.Is(err, qrattendance.ErrSessionExists):
		response.Error(c, http.StatusConflict, "session_exists", "a QR session already exists for this date and type")
	case errors.Is(err, qrattendance.ErrNotFound), database.IsNotFound(err):
		response.Error(c, http.StatusNotFound, "not_found", "QR session or active staff not found")
	case errors.Is(err, qrattendance.ErrSessionClosed):
		response.Error(c, http.StatusConflict, "session_closed", "QR session is closed")
	case errors.Is(err, qrattendance.ErrOutsideWindow):
		response.Error(c, http.StatusConflict, "outside_window", "QR session is outside its attendance window")
	case errors.Is(err, qrattendance.ErrInvalidCode):
		response.Error(c, http.StatusUnprocessableEntity, "invalid_qr", "QR code is invalid or expired")
	case errors.Is(err, qrattendance.ErrAlreadyScanned):
		response.Error(c, http.StatusConflict, "already_scanned", "check-in is already recorded for today")
	case errors.Is(err, qrattendance.ErrCheckInRequired):
		response.Error(c, http.StatusConflict, "check_in_required", "check-in is required before check-out")
	default:
		h.log.Error("qr_attendance_request_failed", "request_id", response.RequestID(c), "error", err)
		response.Error(c, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
