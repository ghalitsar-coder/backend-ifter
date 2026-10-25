package httptransport

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	sanctionrulespkg "github.com/ghalitsar-coder/backend-ifter/internal/sanctionrules"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

type sanctionRuleHandler struct {
	service *sanctionrulespkg.Service
	log     *slog.Logger
}

func newSanctionRuleHandler(service *sanctionrulespkg.Service, log *slog.Logger) *sanctionRuleHandler {
	return &sanctionRuleHandler{service: service, log: log}
}

// sanctionRuleRequest is the POST/PUT payload of docs/API_ADDENDUM.md §6.4
// plus the package spec's level field. min_points uses a pointer so an
// omitted 0-value is distinguishable and rejected as invalid.
type sanctionRuleRequest struct {
	Name        string `json:"name"`
	MinPoints   *int   `json:"min_points"`
	MaxPoints   *int   `json:"max_points"`
	Action      string `json:"action"`
	Level       string `json:"level"`
	Description string `json:"description"`
	IsActive    *bool  `json:"is_active"`
}

type sanctionRulesReplaceRequest struct {
	Rules []sanctionRuleRequest `json:"rules"`
}

type sanctionRuleStatusRequest struct {
	IsActive *bool `json:"is_active"`
}

func (in sanctionRuleRequest) toInput() sanctionrulespkg.RuleInput {
	return sanctionrulespkg.RuleInput{
		Name:        in.Name,
		MinPoints:   in.MinPoints,
		MaxPoints:   in.MaxPoints,
		Action:      in.Action,
		Level:       in.Level,
		Description: in.Description,
		IsActive:    in.IsActive,
	}
}

func (h *sanctionRuleHandler) actor(c *gin.Context) (sanctionrulespkg.Actor, bool) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		h.writeError(c, sanctionrulespkg.ErrUnauthorized)
		return sanctionrulespkg.Actor{}, false
	}
	return sanctionrulespkg.Actor{ID: user.ID, Roles: user.Roles}, true
}

// List handles GET /sanction-rules -> 200 { "data": [...] }.
func (h *sanctionRuleHandler) List(c *gin.Context) {
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

// Get handles GET /sanction-rules/:id -> 200 { "sanction_rule": {...} }.
func (h *sanctionRuleHandler) Get(c *gin.Context) {
	if _, ok := h.actor(c); !ok {
		return
	}
	result, err := h.service.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"sanction_rule": result})
}

// Create handles POST /sanction-rules -> 201 { "sanction_rule": {...} },
// audited as sanction_rule.create.
func (h *sanctionRuleHandler) Create(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request sanctionRuleRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, sanctionrulespkg.ErrInvalidInput)
		return
	}
	result, err := h.service.Create(c.Request.Context(), actor, request.toInput(), response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusCreated, gin.H{"sanction_rule": result})
}

// Update handles PUT /sanction-rules/:id -> 200 { "sanction_rule": {...} },
// audited as sanction_rule.update.
func (h *sanctionRuleHandler) Update(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request sanctionRuleRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.writeError(c, sanctionrulespkg.ErrInvalidInput)
		return
	}
	result, err := h.service.Update(c.Request.Context(), actor, c.Param("id"), request.toInput(), response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"sanction_rule": result})
}

// SetStatus handles PATCH /sanction-rules/:id/status { is_active } ->
// 200 { "sanction_rule": {...} }, audited as sanction_rule.status.
func (h *sanctionRuleHandler) SetStatus(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request sanctionRuleStatusRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.IsActive == nil {
		h.writeError(c, sanctionrulespkg.ErrInvalidInput)
		return
	}
	result, err := h.service.SetStatus(c.Request.Context(), actor, c.Param("id"), *request.IsActive, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, gin.H{"sanction_rule": result})
}

// Delete handles DELETE /sanction-rules/:id -> 204, audited as
// sanction_rule.delete.
func (h *sanctionRuleHandler) Delete(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	if err := h.service.Delete(c.Request.Context(), actor, c.Param("id"), response.RequestID(c)); err != nil {
		h.writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// ReplaceAll handles PUT /sanction-rules {rules:[...]} -> 200 { "data": [...] },
// replacing the whole rule set in one transaction and auditing the swap with
// before/after rule counts (package spec).
func (h *sanctionRuleHandler) ReplaceAll(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request sanctionRulesReplaceRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.Rules == nil {
		h.writeError(c, sanctionrulespkg.ErrInvalidInput)
		return
	}
	inputs := make([]sanctionrulespkg.RuleInput, 0, len(request.Rules))
	for _, item := range request.Rules {
		inputs = append(inputs, item.toInput())
	}
	result, err := h.service.ReplaceAll(c.Request.Context(), actor, inputs, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

func (h *sanctionRuleHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, sanctionrulespkg.ErrUnauthorized):
		c.Header("WWW-Authenticate", "Bearer")
		response.Error(c, http.StatusUnauthorized, "unauthorized", "authentication required")
	case errors.Is(err, sanctionrulespkg.ErrForbidden):
		response.Error(c, http.StatusForbidden, "forbidden", "insufficient permissions")
	case errors.Is(err, sanctionrulespkg.ErrInvalidInput):
		response.Error(c, http.StatusBadRequest, "invalid_request", "invalid request")
	case errors.Is(err, sanctionrulespkg.ErrNotFound):
		response.Error(c, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, sanctionrulespkg.ErrConflict):
		response.Error(c, http.StatusConflict, "conflict", "sanction rule ranges overlap")
	default:
		h.log.Error("sanction_rule_error", "request_id", response.RequestID(c), "error", err)
		response.Error(c, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
