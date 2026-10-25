package httptransport

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	correctionevidencepkg "github.com/ghalitsar-coder/backend-ifter/internal/correctionevidence"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

// correctionEvidenceHandler serves the §6.6 evidence surface for attendance
// corrections. Roles are gated by the AttendanceOps group the routes attach
// to (admin + teacher + duty_teacher); data scoping lives in the service.
type correctionEvidenceHandler struct {
	service *correctionevidencepkg.Service
	log     *slog.Logger
}

func newCorrectionEvidenceHandler(service *correctionevidencepkg.Service, log *slog.Logger) *correctionEvidenceHandler {
	return &correctionEvidenceHandler{service: service, log: log}
}

func (h *correctionEvidenceHandler) actor(c *gin.Context) (correctionevidencepkg.Actor, bool) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		h.writeError(c, correctionevidencepkg.ErrUnauthorized)
		return correctionevidencepkg.Actor{}, false
	}
	return correctionevidencepkg.Actor{ID: user.ID, Roles: user.Roles}, true
}

// Upload handles POST /attendance/records/:id/corrections/evidence (multipart:
// file part "file", optional "note" and "correction_id" fields). The hash of
// the correction itself is produced by the correction flow, never here.
func (h *correctionEvidenceHandler) Upload(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	input, err := h.bindUpload(c)
	if err != nil {
		h.writeError(c, err)
		return
	}
	item, err := h.service.Upload(c.Request.Context(), actor, c.Param("id"), input, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, gin.H{"evidence": item})
}

// List handles GET /attendance/records/:id/corrections/evidence.
func (h *correctionEvidenceHandler) List(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	items, err := h.service.List(c.Request.Context(), actor, c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"data": items})
}

// Download handles GET /attendance/corrections/evidence/:evidenceId — a
// private binary download with the same nosniff/attachment pattern as the
// dispensation documents.
func (h *correctionEvidenceHandler) Download(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	doc, err := h.service.Download(c.Request.Context(), actor, c.Param("evidenceId"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Content-Disposition", `attachment; filename="`+sanitizeFilenameForHeader(doc.Filename)+`"`)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, doc.ContentType, doc.Data)
}

// Delete handles DELETE /attendance/corrections/evidence/:evidenceId (204,
// audited inside the service).
func (h *correctionEvidenceHandler) Delete(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	if err := h.service.Delete(c.Request.Context(), actor, c.Param("evidenceId"), response.RequestID(c)); err != nil {
		h.writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// bindUpload reads the multipart form under the shared imports size limit.
func (h *correctionEvidenceHandler) bindUpload(c *gin.Context) (correctionevidencepkg.UploadInput, error) {
	maxSize := h.service.MaxFileSize()
	if err := c.Request.ParseMultipartForm(maxSize + (1 << 20)); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return correctionevidencepkg.UploadInput{}, correctionevidencepkg.ErrFileTooLarge
		}
		return correctionevidencepkg.UploadInput{}, correctionevidencepkg.ErrInvalidInput
	}
	header, err := c.FormFile("file")
	if err != nil {
		return correctionevidencepkg.UploadInput{}, correctionevidencepkg.ErrInvalidInput
	}
	if header.Size > maxSize {
		return correctionevidencepkg.UploadInput{}, correctionevidencepkg.ErrFileTooLarge
	}
	file, err := header.Open()
	if err != nil {
		return correctionevidencepkg.UploadInput{}, correctionevidencepkg.ErrInvalidInput
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxSize+1))
	if err != nil {
		return correctionevidencepkg.UploadInput{}, correctionevidencepkg.ErrInvalidInput
	}
	if int64(len(data)) > maxSize {
		return correctionevidencepkg.UploadInput{}, correctionevidencepkg.ErrFileTooLarge
	}
	return correctionevidencepkg.UploadInput{
		Filename:     header.Filename,
		ContentType:  header.Header.Get("Content-Type"),
		Data:         data,
		Note:         strings.TrimSpace(c.PostForm("note")),
		CorrectionID: strings.TrimSpace(c.PostForm("correction_id")),
	}, nil
}

func (h *correctionEvidenceHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, correctionevidencepkg.ErrUnauthorized):
		c.Header("WWW-Authenticate", "Bearer")
		response.Error(c, http.StatusUnauthorized, "unauthorized", "authentication required")
	case errors.Is(err, correctionevidencepkg.ErrForbidden):
		response.Error(c, http.StatusForbidden, "forbidden", "insufficient permissions")
	case errors.Is(err, correctionevidencepkg.ErrInvalidInput):
		response.Error(c, http.StatusBadRequest, "invalid_request", "invalid request")
	case errors.Is(err, correctionevidencepkg.ErrNotFound):
		response.Error(c, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, correctionevidencepkg.ErrConflict):
		response.Error(c, http.StatusConflict, "conflict", "evidence conflicts with an existing record")
	case errors.Is(err, correctionevidencepkg.ErrFileTooLarge):
		response.Error(c, http.StatusRequestEntityTooLarge, "file_too_large", "file melebihi batas ukuran")
	default:
		h.log.Error("correction_evidence_error", "request_id", response.RequestID(c), "error", err)
		response.Error(c, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
