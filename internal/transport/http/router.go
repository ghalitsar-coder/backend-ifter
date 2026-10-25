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
	correctionevidencepkg "github.com/ghalitsar-coder/backend-ifter/internal/correctionevidence"
	devicespkg "github.com/ghalitsar-coder/backend-ifter/internal/devices"
	disciplinepkg "github.com/ghalitsar-coder/backend-ifter/internal/discipline"
	dispensationspkg "github.com/ghalitsar-coder/backend-ifter/internal/dispensations"
	dutyrotationpkg "github.com/ghalitsar-coder/backend-ifter/internal/dutyrotation"
	employmenttypepkg "github.com/ghalitsar-coder/backend-ifter/internal/employmenttype"
	importservice "github.com/ghalitsar-coder/backend-ifter/internal/imports"
	"github.com/ghalitsar-coder/backend-ifter/internal/masterdata"
	notificationspkg "github.com/ghalitsar-coder/backend-ifter/internal/notifications"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/metrics"
	reporting "github.com/ghalitsar-coder/backend-ifter/internal/reporting"
	sanctionrulespkg "github.com/ghalitsar-coder/backend-ifter/internal/sanctionrules"
	studentaccountspkg "github.com/ghalitsar-coder/backend-ifter/internal/studentaccounts"
	teacherpresencepkg "github.com/ghalitsar-coder/backend-ifter/internal/teacherpresence"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/response"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Readiness interface {
	Ping(ctx context.Context) error
}

// Role codes. The canonical set lives in the roles table (see migration 000008);
// these constants keep the capability sub-groups below in sync with that seed.
const (
	RoleAdmin       = "admin"
	RoleStaffTU     = "staff_tu"
	RoleTeacher     = "teacher"
	RoleDutyTeacher = "duty_teacher"
	RolePrincipal   = "principal"
	RoleStudent     = "student"
	RoleParent      = "parent"
)

// Capability role sets. Each slice feeds middleware.RequireRoles for the matching
// sub-group in RouteGroups. Server-side data scoping (own-class vs school-wide)
// is enforced inside the domain services; these sets only gate route reachability.
var (
	rolesMasterData        = []string{RoleAdmin, RoleStaffTU}
	rolesAttendanceOps     = []string{RoleAdmin, RoleTeacher, RoleDutyTeacher}
	rolesAttendanceRead    = []string{RoleAdmin, RoleTeacher, RoleDutyTeacher, RolePrincipal}
	rolesDisciplineOps     = []string{RoleAdmin, RoleTeacher, RoleDutyTeacher}
	rolesDisciplineRead    = []string{RoleAdmin, RoleTeacher, RoleDutyTeacher, RolePrincipal}
	rolesReportsRead       = []string{RoleAdmin, RoleStaffTU, RoleTeacher, RoleDutyTeacher, RolePrincipal}
	rolesImports           = []string{RoleAdmin, RoleStaffTU}
	rolesDispensationsRead = []string{RoleAdmin, RoleStaffTU, RoleTeacher, RoleDutyTeacher, RolePrincipal}
	rolesStudent           = []string{RoleStudent}
	rolesParent            = []string{RoleParent}
)

// RouteGroups bundles the capability-scoped Gin sub-groups built once in New().
// Every group already runs Authenticate and the appropriate RequireRoles guard,
// so a feature register file only attaches its endpoints to the right group(s) —
// it never re-wires authentication. See docs/API_ADDENDUM.md for the matrix.
type RouteGroups struct {
	// V1 is the raw, unauthenticated /api/v1 group for features that need a
	// custom prefix or their own middleware stack.
	V1 *gin.RouterGroup
	// Authenticated is every signed-in user (any role, including student).
	Authenticated *gin.RouterGroup
	// Admin is the full administrative surface (user mgmt, settings writes,
	// duty-assignment master, sanction rules, student-account provisioning).
	Admin *gin.RouterGroup
	// MasterData: students/teachers/classes/academic-years/terms/assignments
	// plus employment types. admin + staff_tu.
	MasterData *gin.RouterGroup
	// AttendanceOps: roster/bulk/close/corrections (+ correction evidence).
	// admin + teacher + duty_teacher. Principal is deliberately excluded.
	AttendanceOps *gin.RouterGroup
	// AttendanceRead: history/summary/record detail/settings/holidays.
	// admin + teacher + duty_teacher + principal.
	AttendanceRead *gin.RouterGroup
	// DisciplineOps: record a violation and add a follow-up.
	// admin + teacher + duty_teacher.
	DisciplineOps *gin.RouterGroup
	// DisciplineRead: categories/records/record detail/student summary.
	// admin + teacher + duty_teacher + principal.
	DisciplineRead *gin.RouterGroup
	// ReportsRead: dashboard + attendance/discipline reports and exports.
	// All staff roles (admin, staff_tu, teacher, duty_teacher, principal).
	ReportsRead *gin.RouterGroup
	// Imports: Excel import/export/template jobs. admin + staff_tu.
	Imports *gin.RouterGroup
	// DispensationsRead: dispensation list/detail for every staff role.
	DispensationsRead *gin.RouterGroup
	// My: student self-service (/my/*) — own attendance, discipline,
	// dispensations. student only.
	My *gin.RouterGroup
	// Parent: guardian self-service (/parent/*) — attendance, discipline and
	// dispensations of the children linked to the login. parent only.
	Parent *gin.RouterGroup
}

// FeatureDeps is the forward-declared dependency bundle handed to each
// Register* scaffold. Later feature agents add their typed service to the struct
// (or to their own Register signature) without touching router.go. Log is the
// only field wired from the original scaffold; the pool-backed services below
// are built by New() when the readiness checker is the API pgx pool (production
// passes it; tests that pass a fake skip them and inject through the register
// seams instead). See §6.1 scaffold note.
type FeatureDeps struct {
	Log                *slog.Logger
	TeacherPresence    *teacherpresencepkg.Service
	Dispensations      *dispensationspkg.Service
	DutyRotation       *dutyrotationpkg.Service
	SanctionRules      *sanctionrulespkg.Service
	EmploymentType     *employmenttypepkg.Service
	CorrectionEvidence *correctionevidencepkg.Service
	StudentAccounts    *studentaccountspkg.Service
	Devices            *devicespkg.Service
	Notifications      *notificationspkg.Service
}

// roleGroup derives a RequireRoles-guarded sub-group from an authenticated parent.
func roleGroup(parent *gin.RouterGroup, roles ...string) *gin.RouterGroup {
	group := parent.Group("")
	group.Use(middleware.RequireRoles(roles...))
	return group
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

		// Authenticated parent: every capability sub-group inherits this once.
		authenticated := v1.Group("")
		authenticated.Use(middleware.Authenticate(authService, log))

		groups := RouteGroups{
			V1:                v1,
			Authenticated:     authenticated,
			Admin:             roleGroup(authenticated, RoleAdmin),
			MasterData:        roleGroup(authenticated, rolesMasterData...),
			AttendanceOps:     roleGroup(authenticated, rolesAttendanceOps...),
			AttendanceRead:    roleGroup(authenticated, rolesAttendanceRead...),
			DisciplineOps:     roleGroup(authenticated, rolesDisciplineOps...),
			DisciplineRead:    roleGroup(authenticated, rolesDisciplineRead...),
			ReportsRead:       roleGroup(authenticated, rolesReportsRead...),
			Imports:           roleGroup(authenticated, rolesImports...),
			DispensationsRead: roleGroup(authenticated, rolesDispensationsRead...),
			My:                roleGroup(authenticated, rolesStudent...),
			Parent:            roleGroup(authenticated, rolesParent...),
		}

		// Web-push is built before the discipline handler so a new violation
		// can notify admin/principal subscribers (F9). The remaining
		// FeatureDeps fields are assembled once the API pool is known below.
		deps := FeatureDeps{Log: log}
		if pool, ok := readiness.(*pgxpool.Pool); ok {
			deps.Notifications = notificationspkg.NewService(pool, log, notificationspkg.ConfigFromEnvironment())
		}

		// User management is admin-only (staff_tu is explicitly excluded).
		groups.Admin.GET("/users", handler.ListUsers)
		groups.Admin.POST("/users", middleware.BodyLimit(64<<10), handler.CreateUser)
		groups.Admin.GET("/users/:id", handler.GetUser)
		groups.Admin.PUT("/users/:id/roles", middleware.BodyLimit(64<<10), handler.UpdateUserRoles)
		groups.Admin.PATCH("/users/:id/status", middleware.BodyLimit(64<<10), handler.UpdateUserStatus)
		groups.Admin.POST("/users/:id/credential-reset", handler.IssueCredential)
		groups.Admin.GET("/roles", handler.ListRoles)

		if importService != nil {
			imports := newImportHandler(importService, log)
			uploadLimit := importService.MaxFileSize() + (1 << 20)
			// Imports group: admin + staff_tu.
			groups.Imports.GET("/imports", imports.List)
			groups.Imports.POST("/imports/students", middleware.BodyLimit(uploadLimit), imports.UploadStudents)
			groups.Imports.POST("/imports/teachers", middleware.BodyLimit(uploadLimit), imports.UploadTeachers)
			groups.Imports.GET("/imports/:id", imports.Get)
			groups.Imports.POST("/imports/:id/confirm", imports.Confirm)
			groups.Imports.GET("/import-templates/:entity", imports.Template)
			groups.Imports.GET("/exports/students", imports.ExportStudents)
			groups.Imports.GET("/exports/teachers", imports.ExportTeachers)
		}

		if attendanceService != nil {
			attendance := newAttendanceHandler(attendanceService, log)
			// Attendance operations: admin + teacher + duty_teacher (not principal).
			groups.AttendanceOps.GET("/attendance/roster", attendance.Roster)
			groups.AttendanceOps.POST("/attendance/bulk", middleware.BodyLimit(256<<10), attendance.Save)
			groups.AttendanceOps.POST("/attendance/close", middleware.BodyLimit(64<<10), attendance.Close)
			groups.AttendanceOps.POST("/attendance/records/:id/corrections", middleware.BodyLimit(64<<10), attendance.Correct)
			// Attendance reads: + principal (read-only).
			groups.AttendanceRead.GET("/attendance/history", attendance.History)
			groups.AttendanceRead.GET("/attendance/summary", attendance.Summary)
			groups.AttendanceRead.GET("/attendance/records/:id", attendance.GetRecord)
			groups.AttendanceRead.GET("/attendance/settings", attendance.GetSettings)
			groups.AttendanceRead.GET("/attendance/holidays", attendance.ListHolidays)
			// Reports and exports: all staff (admin, staff_tu, teacher, duty_teacher, principal).
			groups.ReportsRead.GET("/exports/attendance", attendance.Export)
			groups.ReportsRead.GET("/reports/attendance", attendance.Report)
			groups.ReportsRead.GET("/exports/attendance-report", attendance.ExportReport)

			// Attendance settings writes and the holiday calendar are admin-only.
			groups.Admin.PUT("/attendance/settings", middleware.BodyLimit(64<<10), attendance.UpdateSettings)
			groups.Admin.POST("/attendance/holidays", middleware.BodyLimit(64<<10), attendance.CreateHoliday)
			groups.Admin.DELETE("/attendance/holidays/:date", attendance.DeleteHoliday)
			// Duty-assignment master schedule stays admin-only; duty substitution
			// for duty_teacher lives in register_duty_rotation.go.
			groups.Admin.GET("/duty-assignments", attendance.ListDutyAssignments)
			groups.Admin.POST("/duty-assignments", middleware.BodyLimit(64<<10), attendance.CreateDutyAssignment)
			groups.Admin.PUT("/duty-assignments/:id/end", middleware.BodyLimit(64<<10), attendance.EndDutyAssignment)
		}

		if disciplineService != nil {
			disciplineHandler := newDisciplineHandler(disciplineService, log, deps.Notifications)
			// Discipline operations: admin + teacher + duty_teacher (not principal).
			groups.DisciplineOps.POST("/discipline/records", middleware.BodyLimit(64<<10), disciplineHandler.CreateRecord)
			groups.DisciplineOps.POST("/discipline/records/:id/followups", middleware.BodyLimit(64<<10), disciplineHandler.AddFollowup)
			// Discipline reads: + principal (read-only).
			groups.DisciplineRead.GET("/discipline/categories", disciplineHandler.ListCategories)
			groups.DisciplineRead.GET("/discipline/records", disciplineHandler.ListRecords)
			groups.DisciplineRead.GET("/discipline/records/:id", disciplineHandler.GetRecord)
			groups.DisciplineRead.GET("/discipline/students/:id", disciplineHandler.StudentSummary)
			// Discipline reports and exports: all staff.
			groups.ReportsRead.GET("/exports/discipline", disciplineHandler.Export)
			groups.ReportsRead.GET("/reports/discipline", disciplineHandler.Report)

			// Category management and point overrides are admin-only.
			groups.Admin.POST("/discipline/categories", middleware.BodyLimit(64<<10), disciplineHandler.CreateCategory)
			groups.Admin.PUT("/discipline/categories/:id", middleware.BodyLimit(64<<10), disciplineHandler.UpdateCategory)
			groups.Admin.PATCH("/discipline/categories/:id/status", middleware.BodyLimit(64<<10), disciplineHandler.SetCategoryStatus)
			groups.Admin.PATCH("/discipline/records/:id/points", middleware.BodyLimit(64<<10), disciplineHandler.UpdatePoints)
		}

		if reportingService != nil {
			reportingHandler := newReportingHandler(reportingService, log)
			groups.ReportsRead.GET("/dashboard", reportingHandler.Dashboard)
			groups.Admin.GET("/audit-logs", reportingHandler.AuditLogs)
		}

		if masterDataService != nil {
			masterData := newMasterDataHandler(masterDataService, log)
			// Master data CRUD: admin + staff_tu.
			groups.MasterData.GET("/academic-years", masterData.ListAcademicYears)
			groups.MasterData.POST("/academic-years", middleware.BodyLimit(64<<10), masterData.CreateAcademicYear)
			groups.MasterData.GET("/academic-years/:id", masterData.GetAcademicYear)
			groups.MasterData.PUT("/academic-years/:id", middleware.BodyLimit(64<<10), masterData.UpdateAcademicYear)
			groups.MasterData.GET("/academic-years/:id/terms", masterData.ListTerms)
			groups.MasterData.POST("/academic-years/:id/terms", middleware.BodyLimit(64<<10), masterData.CreateTerm)
			groups.MasterData.GET("/terms/:id", masterData.GetTerm)
			groups.MasterData.PUT("/terms/:id", middleware.BodyLimit(64<<10), masterData.UpdateTerm)

			groups.MasterData.GET("/classes", masterData.ListClasses)
			groups.MasterData.POST("/classes", middleware.BodyLimit(64<<10), masterData.CreateClass)
			groups.MasterData.GET("/classes/:id", masterData.GetClass)
			groups.MasterData.PUT("/classes/:id", middleware.BodyLimit(64<<10), masterData.UpdateClass)
			groups.MasterData.PATCH("/classes/:id/status", middleware.BodyLimit(64<<10), masterData.UpdateClassStatus)
			groups.MasterData.GET("/classes/:id/students", masterData.ListClassStudents)
			groups.MasterData.PUT("/classes/:id/homeroom", middleware.BodyLimit(64<<10), masterData.AssignHomeroom)

			groups.MasterData.GET("/teachers", masterData.ListTeachers)
			groups.MasterData.POST("/teachers", middleware.BodyLimit(64<<10), masterData.CreateTeacher)
			groups.MasterData.GET("/teachers/:id", masterData.GetTeacher)
			groups.MasterData.PUT("/teachers/:id", middleware.BodyLimit(64<<10), masterData.UpdateTeacher)
			groups.MasterData.PATCH("/teachers/:id/status", middleware.BodyLimit(64<<10), masterData.UpdateTeacherStatus)

			groups.MasterData.GET("/students", masterData.ListStudents)
			groups.MasterData.POST("/students", middleware.BodyLimit(64<<10), masterData.CreateStudent)
			groups.MasterData.GET("/students/:id", masterData.GetStudent)
			groups.MasterData.PUT("/students/:id", middleware.BodyLimit(64<<10), masterData.UpdateStudent)
			groups.MasterData.PATCH("/students/:id/status", middleware.BodyLimit(64<<10), masterData.UpdateStudentStatus)
			groups.MasterData.PUT("/students/:id/class", middleware.BodyLimit(64<<10), masterData.AssignStudentClass)

			// Administrative notes (FR-C03): staff read/append/delete, never
			// edit. Same admin + staff_tu guard as the rest of master data.
			groups.MasterData.GET("/students/:id/notes", masterData.ListStudentNotes)
			groups.MasterData.POST("/students/:id/notes", middleware.BodyLimit(16<<10), masterData.CreateStudentNote)
			groups.MasterData.DELETE("/students/:id/notes/:noteId", masterData.DeleteStudentNote)
		}

		// Beyond-MVP feature scaffolds. Each Register* is a no-op today; the
		// owning feature agent fills its own file and domain package against the
		// contract in docs/API_ADDENDUM.md. Attach endpoints to the groups above.
		if pool, ok := readiness.(*pgxpool.Pool); ok {
			deps.TeacherPresence = teacherpresencepkg.NewService(pool, log)
			// Dispensations, correction evidence and student accounts share the
			// API pool and the one auth service built at the composition root
			// (no feature-owned pools or reloaded-config token minters).
			// Dispensations share the private imports storage dir and size limit
			// (API_ADDENDUM.md §3); the env fallback mirrors the IFTER_IMPORTS_*
			// overrides, and a live import service wins for the size limit.
			dispensationsCfg := dispensationspkg.ConfigFromEnvironment()
			correctionCfg := correctionevidencepkg.ConfigFromEnvironment()
			if importService != nil {
				dispensationsCfg.MaxFileSize = importService.MaxFileSize()
				correctionCfg.MaxFileSize = importService.MaxFileSize()
			}
			deps.Dispensations = dispensationspkg.NewService(pool, log, dispensationsCfg)
			deps.DutyRotation = dutyrotationpkg.NewService(pool, log)
			deps.SanctionRules = sanctionrulespkg.NewService(pool, log)
			deps.EmploymentType = employmenttypepkg.NewService(pool, log)
			deps.CorrectionEvidence = correctionevidencepkg.NewService(pool, log, correctionCfg)
			deps.StudentAccounts = studentaccountspkg.NewService(pool, authAccountProvisioner{service: authService}, log)
			deps.Devices = devicespkg.NewService(pool, log)
		}
		RegisterTeacherPresence(v1, groups, deps)
		RegisterDispensations(v1, groups, deps)
		RegisterDutyRotation(v1, groups, deps)
		RegisterSanctionRules(v1, groups, deps)
		RegisterEmploymentType(v1, groups, deps)
		RegisterCorrectionEvidence(v1, groups, deps)
		RegisterStudentAccounts(v1, groups, deps)
		RegisterParentPortal(v1, groups, deps)
		RegisterDevices(v1, groups, deps)
		RegisterNotifications(v1, groups, deps)
	}

	return r
}
