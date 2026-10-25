package httptransport

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	employmenttypepkg "github.com/ghalitsar-coder/backend-ifter/internal/employmenttype"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

type employmentTypeHandler struct {
	service *employmenttypepkg.Service
	log     *slog.Logger
}

func newEmploymentTypeHandler(service *employmenttypepkg.Service, log *slog.Logger) *employmentTypeHandler {
	return &employmentTypeHandler{service: service, log: log}
}

// employmentTypeRequest is the POST/PUT payload of docs/API_ADDENDUM.md §6.5:
// { code, name, description?, is_active? }. is_active uses a pointer so an
// omitted field keeps the row's current value on PUT.
type employmentTypeRequest struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsActive    *bool  `json:"is_active"`
}

type employmentTypeStatusRequest struct {
	IsActive *bool `json:"is_active"`
}

func (in employmentTypeRequest) toInput() employmenttypepkg.TypeInput {
	return employmenttypepkg.TypeInput{
		Code:        in.Code,
		Name:        in.Name,
		Description: in.Description,
		IsActive:    in.IsActive,
	}
}

func (h *employmentTypeHandler) actor(c *gin.Context) (employmenttypepkg.Actor, bool) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		h.writeError(c, employmenttypepkg.ErrUnauthorized)
		return employmenttypepkg.Actor{}, false
	}
	return employmenttypepkg.Actor{ID: user.ID, Roles: user.Roles}, true
}

// List handles GET /employment-types -> 200 { "data": [...] }.
func (h *employmentTypeHandler) List(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if _, ok := h.actor(c); !ok {
		return
	}
	result, err := h.service.List(c.Request.Context())
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

// Get handles GET /employment-types/:id -> 200 { "employment_type": {...} }.
func (h *employmentTypeHandler) Get(c *gin.Context) {
	if _, ok := h.actor(c); !ok {
		return
	}
	result, err := h.service.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"employment_type": result})
}

// Create handles POST /employment-types -> 201 { "employment_type": {...} },
// audited as employment_type.create.
func (h *employmentTypeHandler) Create(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request employmentTypeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, employmenttypepkg.ErrInvalidInput)
		return
	}
	result, err := h.service.Create(c.Request.Context(), actor, request.toInput(), response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, gin.H{"employment_type": result})
}

// Update handles PUT /employment-types/:id -> 200 { "employment_type": {...} },
// audited as employment_type.update.
func (h *employmentTypeHandler) Update(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request employmentTypeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, employmenttypepkg.ErrInvalidInput)
		return
	}
	result, err := h.service.Update(c.Request.Context(), actor, c.Param("id"), request.toInput(), response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"employment_type": result})
}

// SetStatus handles PATCH /employment-types/:id/status { is_active } ->
// 200 { "employment_type": {...} }, audited as employment_type.status.
func (h *employmentTypeHandler) SetStatus(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request employmentTypeStatusRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.IsActive == nil {
		h.writeError(c, employmenttypepkg.ErrInvalidInput)
		return
	}
	result, err := h.service.SetStatus(c.Request.Context(), actor, c.Param("id"), *request.IsActive, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"employment_type": result})
}

func (h *employmentTypeHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, employmenttypepkg.ErrUnauthorized):
		c.Header("WWW-Authenticate", "Bearer")
		response.Error(c, http.StatusUnauthorized, "unauthorized", "authentication required")
	case errors.Is(err, employmenttypepkg.ErrForbidden):
		response.Error(c, http.StatusForbidden, "forbidden", "insufficient permissions")
	case errors.Is(err, employmenttypepkg.ErrInvalidInput):
		response.Error(c, http.StatusBadRequest, "invalid_request", "invalid request")
	case errors.Is(err, employmenttypepkg.ErrNotFound):
		response.Error(c, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, employmenttypepkg.ErrConflict):
		response.Error(c, http.StatusConflict, "conflict", "employment type code already exists")
	default:
		h.log.Error("employment_type_error", "request_id", response.RequestID(c), "error", err)
		response.Error(c, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
