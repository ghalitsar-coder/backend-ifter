package httptransport

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	attendancepkg "github.com/ghalitsar-coder/backend-ifter/internal/attendance"
	"github.com/ghalitsar-coder/backend-ifter/internal/auth"
	"github.com/ghalitsar-coder/backend-ifter/internal/config"
	disciplinepkg "github.com/ghalitsar-coder/backend-ifter/internal/discipline"
	importservice "github.com/ghalitsar-coder/backend-ifter/internal/imports"
	"github.com/ghalitsar-coder/backend-ifter/internal/masterdata"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/metrics"
	"github.com/ghalitsar-coder/backend-ifter/internal/reporting"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
)

type Readiness interface {
	Ping(ctx context.Context) error
}

func New(log *slog.Logger, readiness Readiness, authService *auth.Service, masterDataService *masterdata.Service, importService *importservice.Service, attendanceService *attendancepkg.Service, disciplineService *disciplinepkg.Service, reportingService *reporting.Service, authCfg config.AuthConfig, trustedProxies []string) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	if err := r.SetTrustedProxies(trustedProxies); err != nil {
		log.Warn("invalid_trusted_proxies", "error", err)
		_ = r.SetTrustedProxies(nil)
	}
	r.Use(
		middleware.RequestID(),
		middleware.Metrics(),
		middleware.AccessLog(log),
		middleware.Recovery(log),
		middleware.CORS(authCfg.AllowedOrigins),
	)

	r.GET("/healthz", func(c *gin.Context) {
		response.JSON(c, http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/metrics", gin.WrapH(metrics.Handler()))

	r.GET("/readyz", func(c *gin.Context) {
		if readiness == nil {
			response.Error(c, http.StatusServiceUnavailable, "not_ready", "service unavailable")
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := readiness.Ping(ctx); err != nil {
			response.Error(c, http.StatusServiceUnavailable, "not_ready", "service unavailable")
			return
		}
		response.JSON(c, http.StatusOK, gin.H{"status": "ok"})
	})

	if authService != nil {
		handler := newAuthHandler(authService, authCfg, log)
		rateLimit := authCfg.LoginRateLimit
		if rateLimit < 1 {
			rateLimit = 30
		}
		rateWindow := authCfg.LoginRateWindow
		if rateWindow <= 0 {
			rateWindow = time.Minute
		}
		v1 := r.Group("/api/v1")
		authRoutes := v1.Group("/auth")
		authRoutes.POST("/login",
			middleware.BodyLimit(64<<10),
			middleware.LoginRateLimit(rateLimit, rateWindow),
			handler.Login,
		)
		authRoutes.POST("/activate",
			middleware.BodyLimit(64<<10),
			middleware.LoginRateLimit(rateLimit, rateWindow),
			handler.Activate,
		)
		authRoutes.POST("/refresh", handler.Refresh)
		authRoutes.POST("/logout", handler.Logout)
		authRoutes.GET("/me", middleware.Authenticate(authService, log), handler.Me)

		admin := v1.Group("")
		admin.Use(
			middleware.Authenticate(authService, log),
			middleware.RequireRoles("admin"),
		)
		admin.GET("/users", handler.ListUsers)
		admin.POST("/users", middleware.BodyLimit(64<<10), handler.CreateUser)
		admin.GET("/users/:id", handler.GetUser)
		admin.PUT("/users/:id/roles", middleware.BodyLimit(64<<10), handler.UpdateUserRoles)
		admin.PATCH("/users/:id/status", middleware.BodyLimit(64<<10), handler.UpdateUserStatus)
		admin.POST("/users/:id/credential-reset", handler.IssueCredential)
		admin.GET("/roles", handler.ListRoles)

		staff := v1.Group("")
		staff.Use(
			middleware.Authenticate(authService, log),
			middleware.RequireRoles("admin", "teacher", "duty_teacher", "principal"),
		)

		if importService != nil {
			imports := newImportHandler(importService, log)
			uploadLimit := importService.MaxFileSize() + (1 << 20)
			admin.GET("/imports", imports.List)
			admin.POST("/imports/students", middleware.BodyLimit(uploadLimit), imports.UploadStudents)
			admin.POST("/imports/teachers", middleware.BodyLimit(uploadLimit), imports.UploadTeachers)
			admin.GET("/imports/:id", imports.Get)
			admin.POST("/imports/:id/confirm", imports.Confirm)
			admin.GET("/import-templates/:entity", imports.Template)
			admin.GET("/exports/students", imports.ExportStudents)
			admin.GET("/exports/teachers", imports.ExportTeachers)
		}

		if attendanceService != nil {
			attendance := newAttendanceHandler(attendanceService, log)
			staff.GET("/attendance/roster", attendance.Roster)
			staff.POST("/attendance/bulk", middleware.BodyLimit(256<<10), attendance.Save)
			staff.POST("/attendance/close", middleware.BodyLimit(64<<10), attendance.Close)
			staff.GET("/attendance/history", attendance.History)
			staff.GET("/attendance/summary", attendance.Summary)
			staff.GET("/attendance/records/:id", attendance.GetRecord)
			staff.POST("/attendance/records/:id/corrections", middleware.BodyLimit(64<<10), attendance.Correct)
			staff.GET("/exports/attendance", attendance.Export)
			staff.GET("/reports/attendance", attendance.Report)
			staff.GET("/exports/attendance-report", attendance.ExportReport)
			staff.GET("/attendance/settings", attendance.GetSettings)
			staff.GET("/attendance/holidays", attendance.ListHolidays)

			admin.PUT("/attendance/settings", middleware.BodyLimit(64<<10), attendance.UpdateSettings)
			admin.POST("/attendance/holidays", middleware.BodyLimit(64<<10), attendance.CreateHoliday)
			admin.DELETE("/attendance/holidays/:date", attendance.DeleteHoliday)
			admin.GET("/duty-assignments", attendance.ListDutyAssignments)
			admin.POST("/duty-assignments", middleware.BodyLimit(64<<10), attendance.CreateDutyAssignment)
			admin.PUT("/duty-assignments/:id/end", middleware.BodyLimit(64<<10), attendance.EndDutyAssignment)
		}

		if disciplineService != nil {
			disciplineHandler := newDisciplineHandler(disciplineService, log)
			staff.GET("/discipline/categories", disciplineHandler.ListCategories)
			staff.GET("/discipline/records", disciplineHandler.ListRecords)
			staff.POST("/discipline/records", middleware.BodyLimit(64<<10), disciplineHandler.CreateRecord)
			staff.GET("/discipline/records/:id", disciplineHandler.GetRecord)
			staff.POST("/discipline/records/:id/followups", middleware.BodyLimit(64<<10), disciplineHandler.AddFollowup)
			staff.GET("/discipline/students/:id", disciplineHandler.StudentSummary)
			staff.GET("/exports/discipline", disciplineHandler.Export)
			staff.GET("/reports/discipline", disciplineHandler.Report)

			admin.POST("/discipline/categories", middleware.BodyLimit(64<<10), disciplineHandler.CreateCategory)
			admin.PUT("/discipline/categories/:id", middleware.BodyLimit(64<<10), disciplineHandler.UpdateCategory)
			admin.PATCH("/discipline/categories/:id/status", middleware.BodyLimit(64<<10), disciplineHandler.SetCategoryStatus)
			admin.PATCH("/discipline/records/:id/points", middleware.BodyLimit(64<<10), disciplineHandler.UpdatePoints)
		}

		if reportingService != nil {
			reportingHandler := newReportingHandler(reportingService, log)
			staff.GET("/dashboard", reportingHandler.Dashboard)
			admin.GET("/audit-logs", reportingHandler.AuditLogs)
		}

		if masterDataService != nil {
			masterData := newMasterDataHandler(masterDataService, log)
			admin.GET("/academic-years", masterData.ListAcademicYears)
			admin.POST("/academic-years", middleware.BodyLimit(64<<10), masterData.CreateAcademicYear)
			admin.GET("/academic-years/:id", masterData.GetAcademicYear)
			admin.PUT("/academic-years/:id", middleware.BodyLimit(64<<10), masterData.UpdateAcademicYear)
			admin.GET("/academic-years/:id/terms", masterData.ListTerms)
			admin.POST("/academic-years/:id/terms", middleware.BodyLimit(64<<10), masterData.CreateTerm)
			admin.GET("/terms/:id", masterData.GetTerm)
			admin.PUT("/terms/:id", middleware.BodyLimit(64<<10), masterData.UpdateTerm)

			admin.GET("/classes", masterData.ListClasses)
			admin.POST("/classes", middleware.BodyLimit(64<<10), masterData.CreateClass)
			admin.GET("/classes/:id", masterData.GetClass)
			admin.PUT("/classes/:id", middleware.BodyLimit(64<<10), masterData.UpdateClass)
			admin.PATCH("/classes/:id/status", middleware.BodyLimit(64<<10), masterData.UpdateClassStatus)
			admin.GET("/classes/:id/students", masterData.ListClassStudents)
			admin.PUT("/classes/:id/homeroom", middleware.BodyLimit(64<<10), masterData.AssignHomeroom)

			admin.GET("/teachers", masterData.ListTeachers)
			admin.POST("/teachers", middleware.BodyLimit(64<<10), masterData.CreateTeacher)
			admin.GET("/teachers/:id", masterData.GetTeacher)
			admin.PUT("/teachers/:id", middleware.BodyLimit(64<<10), masterData.UpdateTeacher)
			admin.PATCH("/teachers/:id/status", middleware.BodyLimit(64<<10), masterData.UpdateTeacherStatus)

			admin.GET("/students", masterData.ListStudents)
			admin.POST("/students", middleware.BodyLimit(64<<10), masterData.CreateStudent)
			admin.GET("/students/:id", masterData.GetStudent)
			admin.PUT("/students/:id", middleware.BodyLimit(64<<10), masterData.UpdateStudent)
			admin.PATCH("/students/:id/status", middleware.BodyLimit(64<<10), masterData.UpdateStudentStatus)
			admin.PUT("/students/:id/class", middleware.BodyLimit(64<<10), masterData.AssignStudentClass)
		}
	}

	return r
}
