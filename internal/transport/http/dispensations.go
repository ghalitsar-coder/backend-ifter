package httptransport

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	dispensationpkg "github.com/ghalitsar-coder/backend-ifter/internal/dispensations"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

type dispensationHandler struct {
	service *dispensationpkg.Service
	log     *slog.Logger
}

func newDispensationHandler(service *dispensationpkg.Service, log *slog.Logger) *dispensationHandler {
	return &dispensationHandler{service: service, log: log}
}

type dispensationCreateRequest struct {
	StudentID string `json:"student_id"`
	Type      string `json:"type"`
	Reason    string `json:"reason"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

type dispensationVerifyRequest struct {
	Status string `json:"status"`
	Note   string `json:"note"`
}

func (h *dispensationHandler) actor(c *gin.Context) (dispensationpkg.Actor, bool) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		h.writeError(c, dispensationpkg.ErrUnauthorized)
		return dispensationpkg.Actor{}, false
	}
	return dispensationpkg.Actor{ID: user.ID, Roles: user.Roles}, true
}

// List handles GET /dispensations (teachers are scoped to own-class server-side).
func (h *dispensationHandler) List(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	limit, offset, err := parseDispensationPage(c)
	if err != nil {
		h.writeError(c, dispensationpkg.ErrInvalidInput)
		return
	}
	result, err := h.service.List(c.Request.Context(), actor, dispensationpkg.ListFilter{
		Status:    strings.TrimSpace(c.Query("status")),
		Type:      strings.TrimSpace(c.Query("type")),
		StudentID: strings.TrimSpace(c.Query("student_id")),
		ClassID:   strings.TrimSpace(c.Query("class_id")),
		From:      strings.TrimSpace(c.Query("from")),
		To:        strings.TrimSpace(c.Query("to")),
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

// Get handles GET /dispensations/:id -> { "dispensation": { ... } }.
func (h *dispensationHandler) Get(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	item, err := h.service.Get(c.Request.Context(), actor, c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"dispensation": item})
}

// Create handles POST /dispensations, multipart (optional "document" file
// part) or JSON, on behalf of a student.
func (h *dispensationHandler) Create(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	input, err := h.bindCreate(c)
	if err != nil {
		h.writeError(c, err)
		return
	}
	item, err := h.service.Create(c.Request.Context(), actor, input)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, gin.H{"dispensation": item})
}

// CreateMine handles POST /my/dispensations; the student is always the
// signed-in account's own, never a client-supplied id.
func (h *dispensationHandler) CreateMine(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	input, err := h.bindCreate(c)
	if err != nil {
		h.writeError(c, err)
		return
	}
	item, err := h.service.CreateMine(c.Request.Context(), actor, input)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, gin.H{"dispensation": item})
}

// ListMine handles GET /my/dispensations?status=&limit=&offset=.
func (h *dispensationHandler) ListMine(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	limit, offset, err := parseDispensationPage(c)
	if err != nil {
		h.writeError(c, dispensationpkg.ErrInvalidInput)
		return
	}
	result, err := h.service.ListMine(c.Request.Context(), actor, dispensationpkg.ListFilter{
		Status: strings.TrimSpace(c.Query("status")),
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

// Verify handles POST /dispensations/:id/verify {status, note}.
func (h *dispensationHandler) Verify(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request dispensationVerifyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, dispensationpkg.ErrInvalidInput)
		return
	}
	item, err := h.service.Verify(c.Request.Context(), actor, c.Param("id"), dispensationpkg.VerifyInput{
		Status: strings.TrimSpace(request.Status),
		Note:   request.Note,
	}, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"dispensation": item})
}

// Document handles GET /dispensations/:id/document — a private file download
// with the same nosniff/attachment pattern as the xlsx exports.
func (h *dispensationHandler) Document(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	doc, err := h.service.Document(c.Request.Context(), actor, c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Content-Disposition", `attachment; filename="`+sanitizeFilenameForHeader(doc.Filename)+`"`)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, doc.ContentType, doc.Data)
}

// bindCreate accepts both request encodings (docs/API_ADDENDUM.md §6.2):
// multipart form with fields + optional "document" file part, or plain JSON.
func (h *dispensationHandler) bindCreate(c *gin.Context) (dispensationpkg.CreateInput, error) {
	if strings.HasPrefix(strings.ToLower(c.ContentType()), "multipart/form-data") {
		return h.bindCreateMultipart(c)
	}
	var request dispensationCreateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return dispensationpkg.CreateInput{}, dispensationpkg.ErrInvalidInput
	}
	return dispensationpkg.CreateInput{
		StudentID: strings.TrimSpace(request.StudentID),
		Type:      strings.TrimSpace(request.Type),
		Reason:    request.Reason,
		StartDate: strings.TrimSpace(request.StartDate),
		EndDate:   strings.TrimSpace(request.EndDate),
	}, nil
}

func (h *dispensationHandler) bindCreateMultipart(c *gin.Context) (dispensationpkg.CreateInput, error) {
	maxSize := h.service.MaxFileSize()
	if err := c.Request.ParseMultipartForm(maxSize + (1 << 20)); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return dispensationpkg.CreateInput{}, dispensationpkg.ErrFileTooLarge
		}
		return dispensationpkg.CreateInput{}, dispensationpkg.ErrInvalidInput
	}
	input := dispensationpkg.CreateInput{
		StudentID: strings.TrimSpace(c.PostForm("student_id")),
		Type:      strings.TrimSpace(c.PostForm("type")),
		Reason:    c.PostForm("reason"),
		StartDate: strings.TrimSpace(c.PostForm("start_date")),
		EndDate:   strings.TrimSpace(c.PostForm("end_date")),
	}
	header, err := c.FormFile("document")
	if err != nil {
		if !isMissingFormFile(err) {
			return dispensationpkg.CreateInput{}, dispensationpkg.ErrInvalidInput
		}
		return input, nil
	}
	if header.Size > maxSize {
		return dispensationpkg.CreateInput{}, dispensationpkg.ErrFileTooLarge
	}
	file, err := header.Open()
	if err != nil {
		return dispensationpkg.CreateInput{}, dispensationpkg.ErrInvalidInput
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxSize+1))
	if err != nil {
		return dispensationpkg.CreateInput{}, dispensationpkg.ErrInvalidInput
	}
	if int64(len(data)) > maxSize {
		return dispensationpkg.CreateInput{}, dispensationpkg.ErrFileTooLarge
	}
	if len(data) > 0 {
		input.Document = &dispensationpkg.DocumentInput{
			Filename:    header.Filename,
			ContentType: header.Header.Get("Content-Type"),
			Data:        data,
		}
	}
	return input, nil
}

func isMissingFormFile(err error) bool {
	return errors.Is(err, http.ErrMissingFile)
}

func sanitizeFilenameForHeader(value string) string {
	value = strings.ReplaceAll(value, `"`, "")
	value = strings.ReplaceAll(value, "\r", "")
	value = strings.ReplaceAll(value, "\n", "")
	if value == "" {
		return "document"
	}
	return value
}

func parseDispensationPage(c *gin.Context) (int32, int32, error) {
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

func (h *dispensationHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, dispensationpkg.ErrUnauthorized):
		c.Header("WWW-Authenticate", "Bearer")
		response.Error(c, http.StatusUnauthorized, "unauthorized", "authentication required")
	case errors.Is(err, dispensationpkg.ErrForbidden), errors.Is(err, dispensationpkg.ErrAccountNotLinked):
		response.Error(c, http.StatusForbidden, "forbidden", "insufficient permissions")
	case errors.Is(err, dispensationpkg.ErrInvalidInput):
		response.Error(c, http.StatusBadRequest, "invalid_request", "invalid request")
	case errors.Is(err, dispensationpkg.ErrNotFound):
		response.Error(c, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, dispensationpkg.ErrConflict):
		response.Error(c, http.StatusConflict, "conflict", "dispensation conflicts with an existing record")
	case errors.Is(err, dispensationpkg.ErrFileTooLarge):
		response.Error(c, http.StatusRequestEntityTooLarge, "file_too_large", "file melebihi batas ukuran")
	default:
		h.log.Error("dispensation_error", "request_id", response.RequestID(c), "error", err)
		response.Error(c, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
