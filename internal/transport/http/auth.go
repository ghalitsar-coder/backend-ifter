package httptransport

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/auth"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

type authHandler struct {
	service  *auth.LoginService
	register *auth.RegisterService
	log      *slog.Logger
}

func newAuthHandler(service *auth.LoginService, register *auth.RegisterService, log *slog.Logger) *authHandler {
	return &authHandler{service: service, register: register, log: log}
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *authHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "invalid_request", "username and password are required")
		return
	}

	result, err := h.service.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.JSON(c, http.StatusOK, result)
}

type registerRequest struct {
	SchoolName string `json:"school_name" binding:"required"`
	NPSN       string `json:"npsn" binding:"required"`
	Jenjang    string `json:"jenjang" binding:"required"`
	AdminName  string `json:"admin_name" binding:"required"`
	Email      string `json:"email" binding:"required"`
	Phone      string `json:"phone" binding:"required"`
	Password   string `json:"password" binding:"required"`
}

func (h *authHandler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "invalid_request", "all registration fields are required")
		return
	}

	result, err := h.register.Register(c.Request.Context(), auth.RegisterInput{
		SchoolName: req.SchoolName,
		NPSN:       req.NPSN,
		Jenjang:    req.Jenjang,
		AdminName:  req.AdminName,
		Email:      req.Email,
		Phone:      req.Phone,
		Password:   req.Password,
	})
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidRegistration):
			response.Error(c, http.StatusUnprocessableEntity, "invalid_registration", "registration data is invalid")
		case errors.Is(err, auth.ErrEmailTaken):
			response.Error(c, http.StatusConflict, "email_taken", "email is already registered")
		case errors.Is(err, auth.ErrNPSNTaken):
			response.Error(c, http.StatusConflict, "npsn_taken", "npsn is already registered")
		default:
			h.log.Error("register_request_failed", "request_id", response.RequestID(c), "error", err)
			response.Error(c, http.StatusInternalServerError, "internal_error", "internal server error")
		}
		return
	}
	response.JSON(c, http.StatusCreated, result)
}

func (h *authHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		response.Error(c, http.StatusUnauthorized, "invalid_credentials", "username or password is incorrect")
	case errors.Is(err, auth.ErrAccountDisabled):
		response.Error(c, http.StatusForbidden, "account_disabled", "account is disabled")
	case errors.Is(err, auth.ErrNoRole):
		response.Error(c, http.StatusForbidden, "no_role", "account has no role assigned")
	default:
		h.log.Error("login_request_failed", "request_id", response.RequestID(c), "error", err)
		response.Error(c, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
