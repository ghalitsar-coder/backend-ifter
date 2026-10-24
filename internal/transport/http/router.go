package httptransport

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/config"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/metrics"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

// Readiness is satisfied by the database pool.
type Readiness interface {
	Ping(ctx context.Context) error
}

type Deps struct {
	Config    *config.Config
	Logger    *slog.Logger
	Metrics   *metrics.Metrics
	Readiness Readiness
	Tokens    middleware.TokenParser
	Version   string
}

func New(d Deps) *gin.Engine {
	gin.SetMode(ginMode(d.Config.App.Env))
	r := gin.New()
	r.HandleMethodNotAllowed = true

	if err := r.SetTrustedProxies(d.Config.HTTP.TrustedProxies); err != nil {
		d.Logger.Warn("invalid_trusted_proxies", "error", err)
		_ = r.SetTrustedProxies(nil)
	}

	r.Use(
		middleware.RequestID(),
		middleware.Metrics(d.Metrics),
		middleware.AccessLog(d.Logger),
		middleware.Recovery(d.Logger),
		middleware.CORS(d.Config.HTTP.AllowedOrigins),
	)

	r.GET("/healthz", func(c *gin.Context) {
		response.JSON(c, http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/readyz", func(c *gin.Context) {
		if d.Readiness == nil {
			response.Error(c, http.StatusServiceUnavailable, "not_ready", "service is not ready")
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := d.Readiness.Ping(ctx); err != nil {
			d.Logger.Warn("readiness_check_failed", "request_id", response.RequestID(c), "error", err)
			response.Error(c, http.StatusServiceUnavailable, "not_ready", "service is not ready")
			return
		}
		response.JSON(c, http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/metrics", gin.WrapH(d.Metrics.Handler()))
	r.GET("/", func(c *gin.Context) {
		response.JSON(c, http.StatusOK, gin.H{
			"name":    d.Config.App.Name,
			"version": d.Version,
			"env":     d.Config.App.Env,
		})
	})

	v1 := r.Group("/v1")
	v1.Use(
		middleware.BodyLimit(d.Config.HTTP.MaxBodyBytes),
		middleware.RateLimit(d.Config.HTTP.RateLimitRequests, d.Config.HTTP.RateLimitWindow),
	)
	v1.GET("/version", func(c *gin.Context) {
		response.JSON(c, http.StatusOK, gin.H{"version": d.Version})
	})

	// The reference secured group. Delete or rename it when the first real
	// module is added; keep the auth + RBAC wiring.
	if d.Tokens != nil {
		secured := v1.Group("")
		secured.Use(middleware.Authenticate(d.Tokens, d.Logger))

		secured.GET("/admin/ping", middleware.RequireRoles("admin"), func(c *gin.Context) {
			response.JSON(c, http.StatusOK, gin.H{"status": "ok"})
		})
	}

	r.NoRoute(func(c *gin.Context) {
		response.Error(c, http.StatusNotFound, "not_found", "route not found")
	})
	r.NoMethod(func(c *gin.Context) {
		response.Error(c, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	})

	return r
}

func ginMode(env string) string {
	if env == "production" {
		return gin.ReleaseMode
	}
	return gin.DebugMode
}
