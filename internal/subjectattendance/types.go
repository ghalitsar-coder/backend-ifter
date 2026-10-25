package subjectattendance

import (
	"errors"
	"time"
)

var (
	ErrUnauthorized = errors.New("subject attendance authorization required")
	ErrForbidden    = errors.New("insufficient permissions")
	ErrInvalidInput = errors.New("invalid subject attendance input")
	ErrNotFound     = errors.New("subject attendance resource not found")
	ErrConflict     = errors.New("subject attendance conflict")
)

const (
	SessionOpen   = "open"
	SessionClosed = "closed"

	StatusPresent = "present"
	StatusLate    = "late"
	StatusSick    = "sick"
	StatusExcused = "excused"
	StatusAbsent  = "absent"

	RoleAdmin       = "admin"
	RoleStaffTU     = "staff_tu"
	RoleTeacher     = "teacher"
	RoleDutyTeacher = "duty_teacher"
	RolePrincipal   = "principal"

	maxCodeRunes     = 30
	maxNameRunes     = 150
	maxTopicRunes    = 200
	maxNoteRunes     = 500
	maxNotesRunes    = 2000
	maxSessionPeriod = 20
	maxBulkRecords   = 200
	maxRangeDays     = 366
)

func validSessionStatus(value string) bool {
	return value == SessionOpen || value == SessionClosed
}

func validRecordStatus(value string) bool {
	switch value {
	case StatusPresent, StatusLate, StatusSick, StatusExcused, StatusAbsent:
		return true
	default:
		return false
	}
}

// Actor is the authenticated principal. The router gates the groups; the
// service re-checks roles and scopes teachers to their own teaching assignments.
type Actor struct {
	ID    string
	Roles []string
}

// Subject is the external shape of one subject row.
type Subject struct {
	ID          string    `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type SubjectInput struct {
	Code        string
	Name        string
	Description string
	IsActive    *bool
}

type SubjectFilter struct {
	Search   string
	IsActive *bool
}

type SubjectList struct {
	Data []Subject `json:"data"`
}

// TeachingAssignment joins a teacher, a subject and a class.
type TeachingAssignment struct {
	ID          string    `json:"id"`
	TeacherID   string    `json:"teacher_id"`
	TeacherName string    `json:"teacher_name"`
	SubjectID   string    `json:"subject_id"`
	SubjectCode string    `json:"subject_code"`
	SubjectName string    `json:"subject_name"`
	ClassID     string    `json:"class_id"`
	ClassName   string    `json:"class_name"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type TeachingAssignmentInput struct {
	TeacherID string
	SubjectID string
	ClassID   string
	IsActive  *bool
}

type TeachingAssignmentFilter struct {
	TeacherID string
	SubjectID string
	ClassID   string
	IsActive  *bool
}

type TeachingAssignmentList struct {
	Data []TeachingAssignment `json:"data"`
}

// Session is one lesson meeting with its assignment context denormalized for
// the frontend list/detail screens.
type Session struct {
	ID                   string     `json:"id"`
	TeachingAssignmentID string     `json:"teaching_assignment_id"`
	SessionDate          string     `json:"session_date"`
	PeriodStart          int32      `json:"period_start"`
	PeriodEnd            int32      `json:"period_end"`
	Topic                *string    `json:"topic"`
	Notes                *string    `json:"notes"`
	Status               string     `json:"status"`
	TeacherID            string     `json:"teacher_id"`
	TeacherName          string     `json:"teacher_name"`
	SubjectID            string     `json:"subject_id"`
	SubjectCode          string     `json:"subject_code"`
	SubjectName          string     `json:"subject_name"`
	ClassID              string     `json:"class_id"`
	ClassName            string     `json:"class_name"`
	ClosedAt             *time.Time `json:"closed_at"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type SessionInput struct {
	TeachingAssignmentID string
	SessionDate          string
	PeriodStart          int32
	PeriodEnd            int32
	Topic                string
	Notes                string
}

type SessionFilter struct {
	From      string
	To        string
	ClassID   string
	SubjectID string
	TeacherID string
	Status    string
	Limit     int32
	Offset    int32
}

type SessionList struct {
	Data   []Session `json:"data"`
	Total  int64     `json:"total"`
	Limit  int32     `json:"limit"`
	Offset int32     `json:"offset"`
}

type SummaryFilter struct {
	From      string
	To        string
	ClassID   string
	SubjectID string
	TeacherID string
}

// RosterRow is one enrolled student plus their record for the session, if any.
type RosterRow struct {
	StudentID     string  `json:"student_id"`
	StudentNumber string  `json:"student_number"`
	FullName      string  `json:"full_name"`
	EnrollmentID  string  `json:"enrollment_id"`
	Status        *string `json:"status"`
	Note          *string `json:"note"`
}

type Roster struct {
	Data []RosterRow `json:"data"`
}

type RecordInput struct {
	StudentID string
	Status    string
	Note      string
}

type SaveRecordsInput struct {
	Records []RecordInput
}

type SessionRecord struct {
	ID            string    `json:"id"`
	SessionID     string    `json:"session_id"`
	StudentID     string    `json:"student_id"`
	StudentNumber string    `json:"student_number"`
	FullName      string    `json:"full_name"`
	EnrollmentID  string    `json:"enrollment_id"`
	Status        string    `json:"status"`
	Note          *string   `json:"note"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type SessionRecordList struct {
	Data []SessionRecord `json:"data"`
}

// SummaryRow is the recap of one teaching assignment over a date range.
type SummaryRow struct {
	TeachingAssignmentID string  `json:"teaching_assignment_id"`
	ClassID              string  `json:"class_id"`
	ClassName            string  `json:"class_name"`
	SubjectID            string  `json:"subject_id"`
	SubjectCode          string  `json:"subject_code"`
	SubjectName          string  `json:"subject_name"`
	TeacherID            string  `json:"teacher_id"`
	TeacherName          string  `json:"teacher_name"`
	Sessions             int64   `json:"sessions"`
	Records              int64   `json:"records"`
	Present              int64   `json:"present"`
	Late                 int64   `json:"late"`
	Sick                 int64   `json:"sick"`
	Excused              int64   `json:"excused"`
	Absent               int64   `json:"absent"`
	Percentage           float64 `json:"attendance_percentage"`
}

type SummaryTotals struct {
	Sessions   int64   `json:"sessions"`
	Records    int64   `json:"records"`
	Present    int64   `json:"present"`
	Late       int64   `json:"late"`
	Sick       int64   `json:"sick"`
	Excused    int64   `json:"excused"`
	Absent     int64   `json:"absent"`
	Percentage float64 `json:"attendance_percentage"`
}

type Summary struct {
	From   string        `json:"from"`
	To     string        `json:"to"`
	Data   []SummaryRow  `json:"data"`
	Totals SummaryTotals `json:"totals"`
}
