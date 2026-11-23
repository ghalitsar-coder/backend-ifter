package httptransport

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/attendance"
	"github.com/ghalitsar-coder/backend-ifter/internal/auth"
	"github.com/ghalitsar-coder/backend-ifter/internal/config"
	"github.com/ghalitsar-coder/backend-ifter/internal/discipline"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/metrics"
	"github.com/ghalitsar-coder/backend-ifter/internal/qrattendance"
	"github.com/ghalitsar-coder/backend-ifter/internal/recap"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

// Readiness is satisfied by the database pool.
type Readiness interface {
	Ping(ctx context.Context) error
}

type Deps struct {
	Config       *config.Config
	Logger       *slog.Logger
	Metrics      *metrics.Metrics
	Readiness    Readiness
	Tokens       middleware.TokenParser
	Auth         *auth.LoginService
	Register     *auth.RegisterService
	Attendance   *attendance.Service
	Discipline   *discipline.Service
	Recaps       *recap.Service
	QRAttendance *qrattendance.Service
	Version      string
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

	// Public inside /v1, so the body limit and rate limit still apply but
	// authentication does not.
	if d.Auth != nil {
		h := newAuthHandler(d.Auth, d.Register, d.Logger)
		v1.POST("/auth/login", h.Login)
		if d.Register != nil {
			v1.POST("/auth/register", h.Register)
		}
	}

	// Secured group: every attendance and discipline route below requires a signed-in user.
	if d.Tokens != nil {
		secured := v1.Group("")
		secured.Use(middleware.Authenticate(d.Tokens, d.Logger))

		if d.Attendance != nil {
			attendanceHandler := newAttendanceHandler(d.Attendance, d.Logger)
			write := middleware.RequireRoles("guru", "tu", "admin")
			secured.GET("/classes", attendanceHandler.ListClasses)
			secured.POST("/attendance-sessions", write, attendanceHandler.CreateSession)
			secured.GET("/attendance-sessions/:id", attendanceHandler.GetSession)
			secured.PUT("/attendance-sessions/:id/attendances", write, attendanceHandler.Record)
			secured.POST("/attendance-sessions/:id/lock", write, attendanceHandler.Lock)
		}

		if d.Discipline != nil {
			disciplineHandler := newDisciplineHandler(d.Discipline, d.Logger)
			kesiswaanOnly := middleware.RequireRoles("kesiswaan")
			secured.GET("/discipline/types", kesiswaanOnly, disciplineHandler.ListViolationTypes)
			secured.POST("/discipline/types", kesiswaanOnly, disciplineHandler.CreateViolationType)
			secured.PUT("/discipline/types/:id", kesiswaanOnly, disciplineHandler.UpdateViolationType)
			secured.DELETE("/discipline/types/:id", kesiswaanOnly, disciplineHandler.DeleteViolationType)

			secured.GET("/discipline/records", kesiswaanOnly, disciplineHandler.ListViolations)
			secured.POST("/discipline/records", kesiswaanOnly, disciplineHandler.RecordViolation)
			secured.PUT("/discipline/records/:id", kesiswaanOnly, disciplineHandler.UpdateViolation)
			secured.PATCH("/discipline/records/:id/followup", kesiswaanOnly, disciplineHandler.UpdateFollowup)
			secured.DELETE("/discipline/records/:id", kesiswaanOnly, disciplineHandler.DeleteViolation)

			secured.GET("/discipline/inspections", kesiswaanOnly, disciplineHandler.ListInspections)
			secured.POST("/discipline/inspections", kesiswaanOnly, disciplineHandler.CreateInspection)
			secured.PUT("/discipline/inspections/:id", kesiswaanOnly, disciplineHandler.UpdateInspection)
			secured.DELETE("/discipline/inspections/:id", kesiswaanOnly, disciplineHandler.DeleteInspection)

			secured.GET("/discipline/students", kesiswaanOnly, disciplineHandler.ListSchoolStudents)
			secured.GET("/discipline/students/:id/summary", kesiswaanOnly, disciplineHandler.GetStudentSummary)
			secured.GET("/discipline/watchlist", kesiswaanOnly, disciplineHandler.ListWatchlist)
			secured.GET("/discipline/classes/:id/students", kesiswaanOnly, disciplineHandler.ListClassStudents)

			secured.GET("/discipline/counseling", kesiswaanOnly, disciplineHandler.ListCounselingRecords)
			secured.POST("/discipline/counseling", kesiswaanOnly, disciplineHandler.CreateCounselingRecord)
			secured.PUT("/discipline/counseling/:id", kesiswaanOnly, disciplineHandler.UpdateCounselingRecord)
			secured.DELETE("/discipline/counseling/:id", kesiswaanOnly, disciplineHandler.DeleteCounselingRecord)

			secured.GET("/discipline/summons", kesiswaanOnly, disciplineHandler.ListParentSummons)
			secured.POST("/discipline/summons", kesiswaanOnly, disciplineHandler.CreateParentSummons)
			secured.PUT("/discipline/summons/:id", kesiswaanOnly, disciplineHandler.UpdateParentSummons)
			secured.PATCH("/discipline/summons/:id/status", kesiswaanOnly, disciplineHandler.UpdateParentSummonsStatus)
			secured.DELETE("/discipline/summons/:id", kesiswaanOnly, disciplineHandler.DeleteParentSummons)
		}

		secured.GET("/admin/ping", middleware.RequireRoles("admin"), func(c *gin.Context) {
			response.JSON(c, http.StatusOK, gin.H{"status": "ok"})
		})
	}
	if d.Recaps != nil && d.Tokens != nil {
		secured := v1.Group("")
		secured.Use(middleware.Authenticate(d.Tokens, d.Logger))
		recapHandler := newRecapHandler(d.Recaps, d.Logger)
		tu := middleware.RequireRoles("tu", "admin")
		secured.POST("/recaps/students", tu, recapHandler.GenerateStudentRecaps)
		secured.GET("/recaps/students", tu, recapHandler.ListStudentRecaps)
		secured.POST("/recaps/students/:id/verify", tu, recapHandler.VerifyStudentRecap)
		secured.POST("/recaps/staff", tu, recapHandler.GenerateStaffRecaps)
		secured.GET("/recaps/staff", tu, recapHandler.ListStaffRecaps)
		secured.POST("/recaps/staff/:id/verify", tu, recapHandler.VerifyStaffRecap)
	}
	if d.QRAttendance != nil && d.Tokens != nil {
		secured := v1.Group("")
		secured.Use(middleware.Authenticate(d.Tokens, d.Logger))
		qrHandler := newQRAttendanceHandler(d.QRAttendance, d.Logger)
		tu := middleware.RequireRoles("tu")
		secured.POST("/staff/qr-sessions", tu, qrHandler.CreateSession)
		secured.GET("/staff/qr-sessions/active", tu, qrHandler.ActiveSession)
		secured.GET("/staff/qr-sessions/:id/code", tu, qrHandler.CurrentCode)
		secured.POST("/staff/qr-sessions/:id/close", tu, qrHandler.CloseSession)
		secured.POST("/staff/attendance/scan", qrHandler.Scan)
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
