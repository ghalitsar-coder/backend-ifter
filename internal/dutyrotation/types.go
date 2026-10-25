// Package dutyrotation implements the duty-rotation feature package
// (docs/API_ADDENDUM.md §6.3, migration 000011): substitutions for the admin
// duty-assignment master schedule plus the accompanying uniform-note reports.
//
// Layering follows the existing modular monolith (handler -> usecase ->
// repository): this package is the usecase/service layer, persistence goes
// through sqlc-generated queries in internal/db.
package dutyrotation

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
)

// Status domains for duty_rotations (docs/API_ADDENDUM.md §6.3).
const (
	StatusSubstituted = "substituted"
	StatusCancelled   = "cancelled"
)

// Uniform report scope domains (package spec, migration 000011).
const (
	ScopeStudent = "student"
	ScopeTeacher = "teacher"
)

func validScope(value string) bool {
	return value == ScopeStudent || value == ScopeTeacher
}

// maxReasonRunes / maxNoteRunes bound the free-text fields, mirroring the
// character_length CHECK constraints of migration 000011.
const (
	maxReasonRunes = 500
	maxNoteRunes   = 500
)

// Actor is the authenticated principal performing a duty-rotation action.
type Actor struct {
	ID    string
	Roles []string
}

// CreateRotationInput is the POST /duty-rotations payload (external field
// name "date" per docs/API_ADDENDUM.md §6.3; stored as rotation_date).
type CreateRotationInput struct {
	DutyAssignmentID    string
	SubstituteTeacherID string
	Date                string
	Reason              string
}

// CancelInput is the PATCH /duty-rotations/:id/cancel payload. Reason is
// optional: when blank the original substitution reason is kept.
type CancelInput struct {
	Reason string
}

// RotationFilter holds validated query parameters for GET /duty-rotations.
type RotationFilter struct {
	Date    string // YYYY-MM-DD, exact rotation_date match
	Weekday int    // 1..7 (Mon..Sun), matches attendance.DutyAssignmentFilter
}

// RotationItem is one substitution row in the API shape of
// docs/API_ADDENDUM.md §6.3.
type RotationItem struct {
	ID                    string    `json:"id"`
	DutyAssignmentID      string    `json:"duty_assignment_id"`
	RotationDate          string    `json:"rotation_date"`
	Weekday               int       `json:"weekday"`
	OriginalTeacherID     string    `json:"original_teacher_id"`
	OriginalTeacherName   string    `json:"original_teacher_name"`
	SubstituteTeacherID   string    `json:"substitute_teacher_id"`
	SubstituteTeacherName string    `json:"substitute_teacher_name"`
	Status                string    `json:"status"`
	Reason                string    `json:"reason"`
	CreatedAt             time.Time `json:"created_at"`
}

// RotationList is the unpaginated list envelope { "data": [...] }.
type RotationList struct {
	Data []RotationItem `json:"data"`
}

// CreateUniformInput is the POST /uniform-reports payload.
type CreateUniformInput struct {
	ForDate         string
	TeacherID       string
	Scope           string
	TargetStudentID string // optional; only valid with scope=student
	Note            string // optional
}

// UniformItem is one uniform report in the package-spec shape.
type UniformItem struct {
	ID              string    `json:"id"`
	ForDate         string    `json:"for_date"`
	TeacherID       string    `json:"teacher_id"`
	Scope           string    `json:"scope"`
	TargetStudentID *string   `json:"target_student_id"`
	Note            *string   `json:"note"`
	CreatedAt       time.Time `json:"created_at"`
}

// UniformFilter holds validated query parameters for GET /uniform-reports.
type UniformFilter struct {
	Date      string
	TeacherID string
}

// UniformList is the unpaginated list envelope { "data": [...] }.
type UniformList struct {
	Data []UniformItem `json:"data"`
}
