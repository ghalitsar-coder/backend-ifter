package httptransport

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/reporting"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

type reportingHandler struct {
	service *reporting.Service
	log     *slog.Logger
}

func newReportingHandler(service *reporting.Service, log *slog.Logger) *reportingHandler {
	return &reportingHandler{service: service, log: log}
}

func (h *reportingHandler) actor(c *gin.Context) (reporting.Actor, bool) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		h.writeError(c, reporting.ErrUnauthorized)
		return reporting.Actor{}, false
	}
	return reporting.Actor{ID: user.ID, Roles: user.Roles}, true
}

func (h *reportingHandler) Dashboard(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	result, err := h.service.Dashboard(c.Request.Context(), actor, reporting.DashboardFilter{
		Date:    c.Query("date"),
		ClassID: c.Query("class_id"),
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

func (h *reportingHandler) AuditLogs(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	limit, offset, err := parseAttendancePage(c)
	if err != nil {
		h.writeError(c, reporting.ErrInvalidInput)
		return
	}
	result, err := h.service.ListAuditLogs(c.Request.Context(), actor, reporting.AuditLogFilter{
		Action:     c.Query("action"),
		EntityType: c.Query("entity_type"),
		EntityID:   c.Query("entity_id"),
		ActorID:    c.Query("actor_id"),
		From:       c.Query("from"),
		To:         c.Query("to"),
		Limit:      limit,
		Offset:     offset,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

func (h *reportingHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, reporting.ErrUnauthorized):
		response.Error(c, http.StatusUnauthorized, "unauthorized", "authentication required")
	case errors.Is(err, reporting.ErrForbidden):
		response.Error(c, http.StatusForbidden, "forbidden", "insufficient permissions")
	case errors.Is(err, reporting.ErrInvalidInput):
		response.Error(c, http.StatusBadRequest, "invalid_request", "invalid reporting filter")
	default:
		h.log.Error("reporting_request_failed", "error", err)
		response.Error(c, http.StatusInternalServerError, "internal_error", "request failed")
	}
}
