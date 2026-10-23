package httptransport

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/auth"
	"github.com/ghalitsar-coder/backend-ifter/internal/config"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

const refreshCookieName = "ifter_refresh"

type AuthService interface {
	Login(ctx context.Context, login, password, requestID string) (auth.Session, error)
	Refresh(ctx context.Context, rawToken, requestID string) (auth.Session, error)
	Logout(ctx context.Context, rawToken, requestID string) error
	ListUsers(ctx context.Context, status string, limit, offset int32) ([]auth.UserSummary, int64, error)
	CreateUser(ctx context.Context, actorID string, input auth.CreateUserInput, requestID string) (auth.UserAccount, auth.CredentialIssue, error)
	GetUser(ctx context.Context, userID string) (auth.UserAccount, error)
	UpdateUserRoles(ctx context.Context, actorID, userID string, roles []string, requestID string) (auth.UserAccount, error)
	UpdateUserStatus(ctx context.Context, actorID, userID, status, requestID string) (auth.UserAccount, error)
	IssueCredential(ctx context.Context, actorID, userID, requestID string) (auth.CredentialIssue, error)
	Activate(ctx context.Context, rawToken, password, requestID string) error
	ListRoles(ctx context.Context) ([]auth.Role, error)
}

type authHandler struct {
	service        AuthService
	cookieSecure   bool
	allowedOrigins []string
	log            *slog.Logger
}

type loginRequest struct {
	Login    string `json:"login" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type authResponse struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresIn   int64     `json:"expires_in"`
	User        auth.User `json:"user"`
}

type userListResponse struct {
	Data       []auth.UserSummary `json:"data"`
	Total      int64              `json:"total"`
	PageLimit  int32              `json:"limit"`
	PageOffset int32              `json:"offset"`
}

type createUserRequest struct {
	Username string   `json:"username" binding:"required"`
	Email    string   `json:"email"`
	FullName string   `json:"full_name" binding:"required"`
	Roles    []string `json:"roles" binding:"required"`
}

type userRolesRequest struct {
	Roles []string `json:"roles" binding:"required"`
}

type userStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

type activateRequest struct {
	Token    string `json:"token" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func newAuthHandler(service AuthService, cfg config.AuthConfig, log *slog.Logger) *authHandler {
	return &authHandler{
		service:        service,
		cookieSecure:   cfg.CookieSecure,
		allowedOrigins: cfg.AllowedOrigins,
		log:            log,
	}
}

func (h *authHandler) Login(c *gin.Context) {
	if !h.originAllowed(c) {
		response.Error(c, http.StatusForbidden, "forbidden", "origin not allowed")
		return
	}
	c.Header("Cache-Control", "no-store")

	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Login) > 255 || len(req.Password) > 1024 {
		response.Error(c, http.StatusBadRequest, "invalid_request", "login and password are required")
		return
	}

	session, err := h.service.Login(c.Request.Context(), req.Login, req.Password, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	h.setRefreshCookie(c, session.RefreshToken, session.RefreshExpiresAt)
	c.JSON(http.StatusOK, newAuthResponse(session))
}

func (h *authHandler) Refresh(c *gin.Context) {
	if !h.originAllowed(c) {
		response.Error(c, http.StatusForbidden, "forbidden", "origin not allowed")
		return
	}
	c.Header("Cache-Control", "no-store")

	rawToken, err := c.Cookie(refreshCookieName)
	if err != nil {
		h.writeError(c, auth.ErrInvalidRefresh)
		return
	}
	session, err := h.service.Refresh(c.Request.Context(), rawToken, response.RequestID(c))
	if err != nil {
		if errors.Is(err, auth.ErrInvalidRefresh) || errors.Is(err, auth.ErrRefreshReuse) {
			h.clearRefreshCookie(c)
		}
		h.writeError(c, err)
		return
	}
	h.setRefreshCookie(c, session.RefreshToken, session.RefreshExpiresAt)
	c.JSON(http.StatusOK, newAuthResponse(session))
}

func (h *authHandler) Logout(c *gin.Context) {
	if !h.originAllowed(c) {
		response.Error(c, http.StatusForbidden, "forbidden", "origin not allowed")
		return
	}
	h.clearRefreshCookie(c)

	rawToken, err := c.Cookie(refreshCookieName)
	if err == nil {
		if err := h.service.Logout(c.Request.Context(), rawToken, response.RequestID(c)); err != nil {
			h.log.Error("logout_failed", "request_id", response.RequestID(c), "error", err)
			response.Error(c, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
	}
	c.Status(http.StatusNoContent)
}

func (h *authHandler) Me(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		h.writeError(c, auth.ErrUnauthorized)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"user": user})
}

func (h *authHandler) ListUsers(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	limit, offset, status, err := parseUserListQuery(c)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_request", "invalid pagination or status")
		return
	}
	users, total, err := h.service.ListUsers(c.Request.Context(), status, limit, offset)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, userListResponse{
		Data:       users,
		Total:      total,
		PageLimit:  limit,
		PageOffset: offset,
	})
}

func (h *authHandler) CreateUser(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_request", "username, full_name, and roles are required")
		return
	}
	actor, ok := middleware.CurrentUser(c)
	if !ok {
		h.writeError(c, auth.ErrUnauthorized)
		return
	}
	account, credential, err := h.service.CreateUser(c.Request.Context(), actor.ID, auth.CreateUserInput{
		Username: req.Username,
		Email:    req.Email,
		FullName: req.FullName,
		Roles:    req.Roles,
	}, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"user": account, "credential": credential})
}

func (h *authHandler) GetUser(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	account, err := h.service.GetUser(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": account})
}

func (h *authHandler) UpdateUserRoles(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req userRolesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_request", "roles are required")
		return
	}
	actor, ok := middleware.CurrentUser(c)
	if !ok {
		h.writeError(c, auth.ErrUnauthorized)
		return
	}
	account, err := h.service.UpdateUserRoles(c.Request.Context(), actor.ID, c.Param("id"), req.Roles, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": account})
}

func (h *authHandler) UpdateUserStatus(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req userStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_request", "status is required")
		return
	}
	actor, ok := middleware.CurrentUser(c)
	if !ok {
		h.writeError(c, auth.ErrUnauthorized)
		return
	}
	account, err := h.service.UpdateUserStatus(c.Request.Context(), actor.ID, c.Param("id"), req.Status, response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": account})
}

func (h *authHandler) IssueCredential(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	actor, ok := middleware.CurrentUser(c)
	if !ok {
		h.writeError(c, auth.ErrUnauthorized)
		return
	}
	credential, err := h.service.IssueCredential(c.Request.Context(), actor.ID, c.Param("id"), response.RequestID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"credential": credential})
}

func (h *authHandler) ListRoles(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	roles, err := h.service.ListRoles(c.Request.Context())
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": roles})
}

func (h *authHandler) Activate(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var req activateRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Token) > 512 || len(req.Password) > 1024 {
		response.Error(c, http.StatusBadRequest, "invalid_request", "token and password are required")
		return
	}
	if err := h.service.Activate(c.Request.Context(), req.Token, req.Password, response.RequestID(c)); err != nil {
		h.writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *authHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		c.Header("WWW-Authenticate", "Bearer")
		response.Error(c, http.StatusUnauthorized, "invalid_credentials", "invalid username or password")
	case errors.Is(err, auth.ErrInvalidRefresh), errors.Is(err, auth.ErrRefreshReuse):
		c.Header("WWW-Authenticate", "Bearer")
		response.Error(c, http.StatusUnauthorized, "unauthorized", "authentication required")
	case errors.Is(err, auth.ErrUnauthorized):
		c.Header("WWW-Authenticate", "Bearer")
		response.Error(c, http.StatusUnauthorized, "unauthorized", "authentication required")
	case errors.Is(err, auth.ErrForbidden):
		response.Error(c, http.StatusForbidden, "forbidden", "forbidden")
	case errors.Is(err, auth.ErrInvalidInput):
		response.Error(c, http.StatusBadRequest, "invalid_request", "invalid request")
	case errors.Is(err, auth.ErrNotFound):
		response.Error(c, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, auth.ErrConflict):
		response.Error(c, http.StatusConflict, "conflict", "username or email already exists")
	case errors.Is(err, auth.ErrLastAdmin):
		response.Error(c, http.StatusConflict, "last_admin", "at least one active administrator is required")
	case errors.Is(err, auth.ErrUserNotReady):
		response.Error(c, http.StatusConflict, "user_not_ready", "user has not completed activation or is locked")
	case errors.Is(err, auth.ErrInvalidCredential):
		response.Error(c, http.StatusBadRequest, "invalid_token", "invalid or expired token")
	default:
		h.log.Error("auth_error", "request_id", response.RequestID(c), "error", err)
		response.Error(c, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func (h *authHandler) setRefreshCookie(c *gin.Context, token string, expiresAt time.Time) {
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 1 {
		maxAge = 1
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     refreshCookieName,
		Value:    token,
		Path:     "/api/v1/auth",
		MaxAge:   maxAge,
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}

func (h *authHandler) clearRefreshCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     "/api/v1/auth",
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}

func (h *authHandler) originAllowed(c *gin.Context) bool {
	origin := c.GetHeader("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	if strings.EqualFold(u.Host, c.Request.Host) {
		return true
	}
	for _, allowed := range h.allowedOrigins {
		if strings.EqualFold(strings.TrimRight(origin, "/"), allowed) {
			return true
		}
	}
	return false
}

func newAuthResponse(session auth.Session) authResponse {
	expiresIn := int64(math.Ceil(time.Until(session.AccessExpiresAt).Seconds()))
	if expiresIn < 0 {
		expiresIn = 0
	}
	return authResponse{
		AccessToken: session.AccessToken,
		TokenType:   "Bearer",
		ExpiresIn:   expiresIn,
		User:        session.User,
	}
}

func parseUserListQuery(c *gin.Context) (limit, offset int32, status string, err error) {
	limit = 50
	if raw := c.Query("limit"); raw != "" {
		value, parseErr := strconv.ParseInt(raw, 10, 32)
		if parseErr != nil || value < 1 || value > 100 {
			return 0, 0, "", auth.ErrInvalidInput
		}
		limit = int32(value)
	}
	if raw := c.Query("offset"); raw != "" {
		value, parseErr := strconv.ParseInt(raw, 10, 32)
		if parseErr != nil || value < 0 {
			return 0, 0, "", auth.ErrInvalidInput
		}
		offset = int32(value)
	}
	status = strings.TrimSpace(c.Query("status"))
	return limit, offset, status, nil
}
