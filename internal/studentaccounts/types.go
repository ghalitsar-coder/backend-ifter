// Package studentaccounts implements the student-accounts feature package
// (docs/API_ADDENDUM.md §6.7 + package spec, migration 000015): admin
// provisioning of a student's login account and the student's own /my/*
// reads over the students.user_id link.
//
// Layering follows the existing modular monolith (handler -> usecase ->
// repository): this package is the usecase/service layer, persistence goes
// through the sqlc-generated queries in internal/db. Account creation is NOT
// reimplemented here — it delegates to internal/auth (the same machinery that
// backs POST /users and POST /auth/activate), so a provisioned student gets an
// invite credential and sets their own password; no shared password is ever
// issued (docs/API_ADDENDUM.md §0 audit rules).
//
// Every /my/* read resolves the student row from the authenticated subject
// (students.user_id) and then reuses the shared attendance/discipline queries
// with that id forced, so a client can never read another student's data by
// passing an id (TECH_STACK.md §7, PRD FR-A03).
package studentaccounts

import (
	"errors"
	"time"
)

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrInvalidInput = errors.New("invalid input")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	// ErrAlreadyLinked is the 409 for provisioning over a student that already
	// owns a login account (the package spec's idempotency guard).
	ErrAlreadyLinked = errors.New("student already has a login account")
	// ErrStudentNotActive is the 409 for a student that has left the school:
	// /my/* only resolves active student rows, so a login for any other status
	// would be unusable.
	ErrStudentNotActive = errors.New("student is not active")
	// ErrAccountNotLinked is the 403 a student-role login gets when no active
	// student row is linked to it yet (students.user_id is NULL).
	ErrAccountNotLinked = errors.New("no student record is linked to this login account")
)

// RoleStudent is the only role allowed on the /my/* surface; RoleAdmin and
// RoleStaffTU own the provisioning surface (package spec).
const (
	RoleStudent   = "student"
	RoleAdmin     = "admin"
	RoleStaffTU   = "staff_tu"
	studentStatus = "active"
)

// Field bounds mirror the users/student column domains (000002_identity,
// 000003_master_data).
const (
	maxUsernameRunes = 100
	maxFullNameRunes = 150
	maxEmailRunes    = 255
	// maxRangeDays mirrors attendance.maxReportDays so /my/* date windows
	// behave exactly like the staff history/summary endpoints.
	maxRangeDays = 366
	// maxSummaryRecords mirrors discipline.maxExportRecords: the aggregate
	// reads fetch the whole (bounded) record set to total it.
	maxSummaryRecords = 100000
)

// Actor is the authenticated principal performing an action.
type Actor struct {
	ID    string
	Roles []string
}

// ProvisionInput is the POST /students/:id/account payload. Username is
// optional: when empty the student's own student_number becomes the username
// (package spec).
type ProvisionInput struct {
	Username string
	Email    string
}

// NewUserInput is what this package asks the Provisioner (internal/auth) to
// create. Roles is always ["student"] here.
type NewUserInput struct {
	Username string
	Email    string
	FullName string
	Roles    []string
}

// ProvisionedAccount is what a Provisioner hands back: the created account and
// the one-time activation credential.
type ProvisionedAccount struct {
	User       UserAccount
	Credential CredentialIssue
}

// ProvisionResult is the 201 body { "user": ..., "credential": ... }. The user
// and credential shapes are reused verbatim from internal/auth so the response
// matches POST /users (docs/API_ADDENDUM.md §6.7).
type ProvisionResult struct {
	User       UserAccount     `json:"user"`
	Credential CredentialIssue `json:"credential"`
}

// UserAccount mirrors auth.UserAccount (auth.UserSummary + roles).
type UserAccount struct {
	ID          string     `json:"id"`
	Username    string     `json:"username"`
	Email       string     `json:"email,omitempty"`
	FullName    string     `json:"full_name"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	Roles       []string   `json:"roles"`
}

// CredentialIssue mirrors auth.CredentialIssue: the opaque one-time activation
// token, its purpose and its expiry. Never persisted or logged in clear text.
type CredentialIssue struct {
	Token     string    `json:"token"`
	Purpose   string    `json:"purpose"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Account is the GET /students/:id/account payload: the login link of one
// student, with every field null while no account has been provisioned.
type Account struct {
	UserID      *string    `json:"user_id"`
	Username    *string    `json:"username"`
	Status      *string    `json:"status"`
	LastLoginAt *time.Time `json:"last_login_at"`
}

// AccountEnvelope is the single-resource shape { "account": {...} }.
type AccountEnvelope struct {
	Account Account `json:"account"`
}

// Profile is GET /my/profile: the student row the login is linked to. It
// exists because auth.User (GET /auth/me) belongs to the auth package and
// cannot carry a student id (see the package deviation notes).
type Profile struct {
	StudentID     string  `json:"student_id"`
	StudentNumber string  `json:"student_number"`
	FullName      string  `json:"full_name"`
	ClassID       *string `json:"class_id"`
	ClassName     *string `json:"class_name"`
}

// ProfileEnvelope is the single-resource shape { "profile": {...} }.
type ProfileEnvelope struct {
	Profile Profile `json:"profile"`
}

// MyAttendanceItem is one /my/attendance row (docs/API_ADDENDUM.md §6.7).
type MyAttendanceItem struct {
	Date        string     `json:"date"`
	Status      string     `json:"status"`
	CheckInTime *time.Time `json:"check_in_time"`
	ClassID     string     `json:"class_id"`
	ClassName   string     `json:"class_name"`
}

// MyAttendanceList is the paginated list envelope of /my/attendance.
type MyAttendanceList struct {
	Data   []MyAttendanceItem `json:"data"`
	Total  int64              `json:"total"`
	Limit  int32              `json:"limit"`
	Offset int32              `json:"offset"`
}

// MyAttendanceDay mirrors attendance.StudentSummary so the student portal can
// reuse the staff summary rendering code unchanged (package spec: reuse the
// summary query).
type MyAttendanceDay struct {
	StudentID     string  `json:"student_id"`
	StudentNumber string  `json:"student_number"`
	StudentName   string  `json:"student_name"`
	ClassID       string  `json:"class_id"`
	ClassName     string  `json:"class_name"`
	TotalDays     int32   `json:"total_days"`
	Present       int32   `json:"present"`
	Late          int32   `json:"late"`
	Sick          int32   `json:"sick"`
	Excused       int32   `json:"excused"`
	Absent        int32   `json:"absent"`
	Percentage    float64 `json:"attendance_percentage"`
}

// MyAttendanceTotals mirrors attendance.ReportTotals.
type MyAttendanceTotals struct {
	Students   int     `json:"students"`
	TotalDays  int32   `json:"total_days"`
	Present    int32   `json:"present"`
	Late       int32   `json:"late"`
	Sick       int32   `json:"sick"`
	Excused    int32   `json:"excused"`
	Absent     int32   `json:"absent"`
	Percentage float64 `json:"attendance_percentage"`
}

// MyAttendanceSummary is the /my/attendance-summary body, shaped like
// GET /attendance/summary ({from,to,data,totals}).
type MyAttendanceSummary struct {
	From   string             `json:"from"`
	To     string             `json:"to"`
	Data   []MyAttendanceDay  `json:"data"`
	Totals MyAttendanceTotals `json:"totals"`
}

// MyDisciplineItem is one /my/discipline row (docs/API_ADDENDUM.md §6.7).
type MyDisciplineItem struct {
	ID           string    `json:"id"`
	OccurredAt   time.Time `json:"occurred_at"`
	CategoryName string    `json:"category_name"`
	Description  string    `json:"description"`
	Points       int32     `json:"points"`
	Status       string    `json:"status"`
}

// MyDisciplineList is the paginated list envelope of /my/discipline.
type MyDisciplineList struct {
	Data   []MyDisciplineItem `json:"data"`
	Total  int64              `json:"total"`
	Limit  int32              `json:"limit"`
	Offset int32              `json:"offset"`
}

// MyDisciplineCategory mirrors discipline.CategorySummary.
type MyDisciplineCategory struct {
	CategoryID   string `json:"category_id"`
	CategoryName string `json:"category_name"`
	Count        int    `json:"count"`
	Points       int32  `json:"points"`
}

// MyDisciplineLevel mirrors discipline.SanctionLevel (the sanction-rules tier
// covering the accumulated points; null when no active rule matches).
type MyDisciplineLevel struct {
	Level  *string `json:"level"`
	Action string  `json:"action"`
}

// MyDisciplineSummary is /my/discipline-summary: the same aggregate the staff
// surface GET /discipline/students/:id returns (package spec: reuse the
// student summary), without the record list.
type MyDisciplineSummary struct {
	StudentID     string                 `json:"student_id"`
	StudentNumber string                 `json:"student_number"`
	StudentName   string                 `json:"student_name"`
	TotalPoints   int32                  `json:"total_points"`
	TotalRecords  int                    `json:"total_records"`
	Categories    []MyDisciplineCategory `json:"categories"`
	CurrentLevel  *MyDisciplineLevel     `json:"current_level"`
}

// DateRangeFilter is the shared from/to (+ pagination) filter of the /my/*
// reads. from and to are required, exactly like GET /attendance/history.
type DateRangeFilter struct {
	From   string
	To     string
	Limit  int32
	Offset int32
}
