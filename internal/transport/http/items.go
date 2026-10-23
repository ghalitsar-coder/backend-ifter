package httptransport

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ghalitsar-coder/backend-ifter/internal/item"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

// itemHandler is the reference HTTP adapter. Handlers only translate transport
// concerns (decode, validate shape, map errors); business rules live in the
// service layer.
type itemHandler struct {
	service *item.Service
	log     *slog.Logger
}

func newItemHandler(service *item.Service, log *slog.Logger) *itemHandler {
	return &itemHandler{service: service, log: log}
}

type createItemRequest struct {
	Name string `json:"name" binding:"required"`
}

func (h *itemHandler) Create(c *gin.Context) {
	var req createItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			response.Error(c, http.StatusRequestEntityTooLarge, "payload_too_large", "request body is too large")
			return
		}
		response.Error(c, http.StatusUnprocessableEntity, "invalid_request", "name is required")
		return
	}

	created, err := h.service.Create(c.Request.Context(), item.CreateInput{Name: req.Name})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, created)
}

func (h *itemHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_id", "id must be a valid UUID")
		return
	}

	found, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, found)
}

func (h *itemHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))

	result, err := h.service.List(c.Request.Context(), item.ListInput{Page: page, PerPage: perPage})
	if err != nil {
		h.writeError(c, err)
		return
	}

	total := result.Total
	response.JSONMeta(c, http.StatusOK, result.Items, &response.Meta{
		Page:    result.Page,
		PerPage: result.PerPage,
		Total:   &total,
	})
}

func (h *itemHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, item.ErrNotFound):
		response.Error(c, http.StatusNotFound, "not_found", "item not found")
	case errors.Is(err, item.ErrInvalidName):
		response.Error(c, http.StatusUnprocessableEntity, "invalid_name", "name must be between 1 and 120 characters")
	default:
		h.log.Error("item_request_failed", "request_id", response.RequestID(c), "error", err)
		response.Error(c, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
