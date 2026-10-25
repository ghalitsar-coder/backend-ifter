package httptransport

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	studentaccountspkg "github.com/ghalitsar-coder/backend-ifter/internal/studentaccounts"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

type studentAccountHandler struct {
	service *studentaccountspkg.Service
	log     *slog.Logger
}

func newStudentAccountHandler(service *studentaccountspkg.Service, log *slog.Logger) *studentAccountHandler {
	return &studentAccountHandler{service: service, log: log}
}

// provisionStudentAccountRequest is the POST /students/:id/account payload
// (docs/API_ADDENDUM.md §6.7). Both fields are optional: username defaults to
// the student's own student_number and full_name always comes from the student
// row (package spec).
type provisionStudentAccountRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
}

func (h *studentAccountHandler) actor(c *gin.Context) (studentaccountspkg.Actor, bool) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		h.writeError(c, studentaccountspkg.ErrUnauthorized)
		return studentaccountspkg.Actor{}, false
	}
	return studentaccountspkg.Actor{ID: user.ID, Roles: user.Roles}, true
}

// Provision handles POST /students/:id/account -> 201
// { "user": {...}, "credential": {...} }, audited as student_account.create.
func (h *studentAccountHandler) Provision(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var request provisionStudentAccountRequest
	// An absent body is valid: the username falls back to the student number.
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&request); err != nil {
			h.writeError(c, studentaccountspkg.ErrInvalidInput)
			return
		}
	}
	result, err := h.service.Provision(c.Request.Context(), actor, c.Param("id"),
		studentaccountspkg.ProvisionInput{Username: request.Username, Email: request.Email},
		response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.JSON(c, http.StatusCreated, result)
}

// GetAccount handles GET /students/:id/account -> 200 { "account": {...} }.
func (h *studentAccountHandler) GetAccount(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	c.Header("Cache-Control", "no-store")
	account, err := h.service.GetAccount(c.Request.Context(), actor, c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, studentaccountspkg.AccountEnvelope{Account: account})
}

// Deactivate handles DELETE /students/:id/account -> 204, audited as
// student_account.delete. The login is withdrawn; the student row and its
// history stay untouched.
func (h *studentAccountHandler) Deactivate(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	if err := h.service.Deactivate(c.Request.Context(), actor, c.Param("id"), response.RequestID(c)); err != nil {
		h.writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// MyProfile handles GET /my/profile -> 200 { "profile": {...} }: the student
// row the login is linked to (the student id a portal needs, since the
// auth-owned GET /auth/me payload cannot carry it).
func (h *studentAccountHandler) MyProfile(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	c.Header("Cache-Control", "no-store")
	result, err := h.service.MyProfile(c.Request.Context(), actor)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

// MyAttendance handles GET /my/attendance?from=&to=&limit=&offset= ->
// 200 { "data": [...], "total", "limit", "offset" } (docs/API_ADDENDUM.md
// §6.7). It is also mounted as GET /my/attendance-history (package spec name).
func (h *studentAccountHandler) MyAttendance(c *gin.Context) {
	actor, filter, ok := h.myFilter(c)
	if !ok {
		return
	}
	c.Header("Cache-Control", "no-store")
	result, err := h.service.MyAttendance(c.Request.Context(), actor, filter)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

// MyAttendanceSummary handles GET /my/attendance-summary?from=&to= ->
// 200 { "from", "to", "data": [...], "totals": {...} }, the same aggregate the
// staff summary endpoint returns.
func (h *studentAccountHandler) MyAttendanceSummary(c *gin.Context) {
	actor, filter, ok := h.myFilter(c)
	if !ok {
		return
	}
	c.Header("Cache-Control", "no-store")
	result, err := h.service.MyAttendanceSummary(c.Request.Context(), actor, filter)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

// MyDiscipline handles GET /my/discipline?from=&to=&limit=&offset= ->
// 200 { "data": [...], "total", "limit", "offset" }.
func (h *studentAccountHandler) MyDiscipline(c *gin.Context) {
	actor, filter, ok := h.myFilter(c)
	if !ok {
		return
	}
	c.Header("Cache-Control", "no-store")
	result, err := h.service.MyDiscipline(c.Request.Context(), actor, filter)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

// MyDisciplineSummary handles GET /my/discipline-summary?from=&to= -> 200 the
// student's own discipline aggregate (points, per-category, current level).
func (h *studentAccountHandler) MyDisciplineSummary(c *gin.Context) {
	actor, filter, ok := h.myFilter(c)
	if !ok {
		return
	}
	c.Header("Cache-Control", "no-store")
	result, err := h.service.MyDisciplineSummary(c.Request.Context(), actor, filter)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

func (h *studentAccountHandler) myFilter(c *gin.Context) (studentaccountspkg.Actor, studentaccountspkg.DateRangeFilter, bool) {
	actor, ok := h.actor(c)
	if !ok {
		return actor, studentaccountspkg.DateRangeFilter{}, false
	}
	limit, offset, err := parseStudentPortalPage(c)
	if err != nil {
		h.writeError(c, studentaccountspkg.ErrInvalidInput)
		return actor, studentaccountspkg.DateRangeFilter{}, false
	}
	return actor, studentaccountspkg.DateRangeFilter{
		From:   c.Query("from"),
		To:     c.Query("to"),
		Limit:  limit,
		Offset: offset,
	}, true
}

// parseStudentPortalPage mirrors parseAttendancePage: default 50, cap 100.
func parseStudentPortalPage(c *gin.Context) (int32, int32, error) {
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

func (h *studentAccountHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, studentaccountspkg.ErrUnauthorized):
		c.Header("WWW-Authenticate", "Bearer")
		response.Error(c, http.StatusUnauthorized, "unauthorized", "authentication required")
	case errors.Is(err, studentaccountspkg.ErrAccountNotLinked):
		response.Error(c, http.StatusForbidden, "forbidden", "no active student record is linked to this login account")
	case errors.Is(err, studentaccountspkg.ErrChildNotLinked):
		response.Error(c, http.StatusNotFound, "not_found", "child is not linked to this login account")
	case errors.Is(err, studentaccountspkg.ErrParentRoleRequired):
		response.Error(c, http.StatusConflict, "conflict", "assign the parent role to this user before linking children")
	case errors.Is(err, studentaccountspkg.ErrForbidden):
		response.Error(c, http.StatusForbidden, "forbidden", "insufficient permissions")
	case errors.Is(err, studentaccountspkg.ErrInvalidInput):
		response.Error(c, http.StatusBadRequest, "invalid_request", "invalid request")
	case errors.Is(err, studentaccountspkg.ErrNotFound):
		response.Error(c, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, studentaccountspkg.ErrAlreadyLinked):
		response.Error(c, http.StatusConflict, "conflict", "this student already has a login account")
	case errors.Is(err, studentaccountspkg.ErrStudentNotActive):
		response.Error(c, http.StatusConflict, "conflict", "only an active student can be given a login account")
	case errors.Is(err, studentaccountspkg.ErrConflict):
		response.Error(c, http.StatusConflict, "conflict", "username or email is already taken")
	default:
		h.log.Error("student_account_error", "request_id", response.RequestID(c), "error", err)
		response.Error(c, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
