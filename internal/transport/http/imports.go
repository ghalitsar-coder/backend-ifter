package httptransport

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	importservice "github.com/ghalitsar-coder/backend-ifter/internal/imports"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

const excelContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

type importHandler struct {
	service *importservice.Service
	log     *slog.Logger
}

func newImportHandler(service *importservice.Service, log *slog.Logger) *importHandler {
	return &importHandler{service: service, log: log}
}

func (h *importHandler) UploadStudents(c *gin.Context) {
	h.upload(c, importservice.EntityStudents)
}

func (h *importHandler) UploadTeachers(c *gin.Context) {
	h.upload(c, importservice.EntityTeachers)
}

func (h *importHandler) upload(c *gin.Context, entity importservice.Entity) {
	c.Header("Cache-Control", "no-store")
	maxSize := h.service.MaxFileSize()
	if err := c.Request.ParseMultipartForm(maxSize + (1 << 20)); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			response.Error(c, http.StatusRequestEntityTooLarge, "file_too_large", "file melebihi batas ukuran")
			return
		}
		response.Error(c, http.StatusBadRequest, "invalid_request", "file wajib diunggah sebagai multipart")
		return
	}
	header, err := c.FormFile("file")
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_request", "file wajib diunggah")
		return
	}
	if header.Size > maxSize {
		response.Error(c, http.StatusRequestEntityTooLarge, "file_too_large", "file melebihi batas ukuran")
		return
	}
	file, err := header.Open()
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_request", "file tidak dapat dibaca")
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxSize+1))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_request", "file tidak dapat dibaca")
		return
	}
	if int64(len(data)) > maxSize {
		response.Error(c, http.StatusRequestEntityTooLarge, "file_too_large", "file melebihi batas ukuran")
		return
	}
	mode := importservice.Mode(strings.TrimSpace(c.PostForm("mode")))
	if mode == "" {
		mode = importservice.ModeCreateOnly
	}
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	result, err := h.service.Upload(c.Request.Context(), actor, entity, mode, header.Filename, data, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, result)
}

func (h *importHandler) List(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	limit, offset, err := parseImportPage(c)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_request", "invalid pagination")
		return
	}
	jobs, total, err := h.service.List(c.Request.Context(), importservice.ListFilter{
		EntityType: strings.TrimSpace(c.Query("entity_type")),
		Status:     strings.TrimSpace(c.Query("status")),
		Limit:      limit,
		Offset:     offset,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": jobs, "total": total, "limit": limit, "offset": offset})
}

func (h *importHandler) Get(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	detail, err := h.service.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, detail)
}

func (h *importHandler) Confirm(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	job, err := h.service.Confirm(c.Request.Context(), actor, c.Param("id"), response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"import": job})
}

func (h *importHandler) Template(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	entity := importservice.Entity(strings.TrimSpace(c.Param("entity")))
	data, err := h.service.Template(entity)
	if err != nil {
		h.writeError(c, err)
		return
	}
	filename := "template-guru.xlsx"
	if entity == importservice.EntityStudents {
		filename = "template-siswa.xlsx"
	}
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, excelContentType, data)
}

func (h *importHandler) ExportStudents(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	data, err := h.service.ExportStudents(c.Request.Context(), importservice.ExportFilter{
		Status:  strings.TrimSpace(c.Query("status")),
		ClassID: strings.TrimSpace(c.Query("class_id")),
		Search:  strings.TrimSpace(c.Query("search")),
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="data-siswa-`+time.Now().Format("20060102")+`.xlsx"`)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, excelContentType, data)
}

func (h *importHandler) ExportTeachers(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	data, err := h.service.ExportTeachers(c.Request.Context(), importservice.ExportFilter{
		Status: strings.TrimSpace(c.Query("status")),
		Search: strings.TrimSpace(c.Query("search")),
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="data-guru-`+time.Now().Format("20060102")+`.xlsx"`)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, excelContentType, data)
}

func (h *importHandler) actor(c *gin.Context) (string, bool) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		response.Error(c, http.StatusUnauthorized, "unauthorized", "authentication required")
		return "", false
	}
	return user.ID, true
}

func (h *importHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, importservice.ErrInvalidInput):
		response.Error(c, http.StatusBadRequest, "invalid_request", "invalid request")
	case errors.Is(err, importservice.ErrNotFound):
		response.Error(c, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, importservice.ErrConflict):
		response.Error(c, http.StatusConflict, "conflict", "import tidak dapat diproses pada status saat ini")
	case errors.Is(err, importservice.ErrPreviewChanged):
		response.Error(c, http.StatusConflict, "preview_changed", "data berubah sejak preview; unggah ulang file")
	case errors.Is(err, importservice.ErrFileTooLarge):
		response.Error(c, http.StatusRequestEntityTooLarge, "file_too_large", "file melebihi batas ukuran")
	default:
		h.log.Error("import_error", "request_id", response.RequestID(c), "error", err)
		response.Error(c, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func parseImportPage(c *gin.Context) (int32, int32, error) {
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
