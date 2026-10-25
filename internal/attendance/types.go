package attendance

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

type Actor struct {
	ID    string
	Roles []string
}

type Settings struct {
	SchoolName    string    `json:"school_name"`
	Timezone      string    `json:"timezone"`
	AutoCloseTime string    `json:"attendance_auto_close_time"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Schedule struct {
	Weekday              int16  `json:"weekday"`
	StartTime            string `json:"start_time"`
	LateToleranceMinutes int16  `json:"late_tolerance_minutes"`
}

type SettingsDetail struct {
	Settings
	Schedules []Schedule `json:"schedules"`
	Holidays  []Holiday  `json:"holidays"`
}

type Holiday struct {
	Date      string    `json:"holiday_date"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type SettingsInput struct {
	SchoolName    string
	Timezone      string
	AutoCloseTime string
	Schedules     []ScheduleInput
}

type ScheduleInput struct {
	Weekday              int16
	StartTime            string
	LateToleranceMinutes int16
}

type HolidayInput struct {
	Date string
	Name string
}

type DutyAssignment struct {
	ID          string    `json:"id"`
	TeacherID   string    `json:"teacher_id"`
	TeacherName string    `json:"teacher_name"`
	Weekday     int16     `json:"weekday"`
	StartDate   string    `json:"start_date"`
	EndDate     *string   `json:"end_date,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type DutyAssignmentInput struct {
	TeacherID string
	Weekday   int16
	StartDate string
	EndDate   string
}

type DutyAssignmentFilter struct {
	TeacherID string
	Weekday   int16
	Date      string
}

type ClassRef struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	AcademicYearID   string `json:"academic_year_id"`
	AcademicYearName string `json:"academic_year_name"`
}

type Closure struct {
	ID           string    `json:"id"`
	ClassID      string    `json:"class_id"`
	Date         string    `json:"date"`
	CloseType    string    `json:"close_type"`
	ClosedByID   string    `json:"closed_by_user_id"`
	ClosedByName string    `json:"closed_by_name"`
	ClosedAt     time.Time `json:"closed_at"`
	AbsentFilled int32     `json:"absent_filled"`
}

type Record struct {
	ID               string     `json:"id"`
	StudentID        string     `json:"student_id"`
	EnrollmentID     string     `json:"enrollment_id"`
	Date             string     `json:"date"`
	Status           string     `json:"status"`
	CheckInAt        *time.Time `json:"check_in_at,omitempty"`
	Notes            string     `json:"notes,omitempty"`
	Source           string     `json:"source"`
	RecordedByUserID string     `json:"recorded_by_user_id"`
	UpdatedByUserID  string     `json:"updated_by_user_id"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	StudentNumber    string     `json:"student_number,omitempty"`
	StudentName      string     `json:"student_name,omitempty"`
	ClassID          string     `json:"class_id,omitempty"`
	ClassName        string     `json:"class_name,omitempty"`
}

type RosterStudent struct {
	StudentID     string  `json:"student_id"`
	StudentNumber string  `json:"student_number"`
	FullName      string  `json:"full_name"`
	EnrollmentID  string  `json:"enrollment_id"`
	Record        *Record `json:"record,omitempty"`
}

type Summary struct {
	Total      int `json:"total"`
	Recorded   int `json:"recorded"`
	Unrecorded int `json:"unrecorded"`
	Present    int `json:"present"`
	Late       int `json:"late"`
	Sick       int `json:"sick"`
	Excused    int `json:"excused"`
	Absent     int `json:"absent"`
}

type Roster struct {
	Class     ClassRef        `json:"class"`
	Date      string          `json:"date"`
	Effective bool            `json:"effective"`
	Closure   *Closure        `json:"closure,omitempty"`
	Summary   Summary         `json:"summary"`
	Students  []RosterStudent `json:"students"`
}

type CloseResult struct {
	Closure Closure `json:"closure"`
	Summary Summary `json:"summary"`
}

type RecordInput struct {
	StudentID string
	Status    string
	CheckInAt string
	Notes     string
}

type SaveInput struct {
	ClassID string
	Date    string
	Records []RecordInput
}

type CorrectionInput struct {
	Status    *string
	CheckInAt *string
	Notes     *string
	Reason    string
}

type Correction struct {
	ID            string     `json:"id"`
	RecordID      string     `json:"attendance_record_id"`
	OldStatus     string     `json:"old_status"`
	NewStatus     string     `json:"new_status"`
	OldCheckInAt  *time.Time `json:"old_check_in_at,omitempty"`
	NewCheckInAt  *time.Time `json:"new_check_in_at,omitempty"`
	OldNotes      string     `json:"old_notes,omitempty"`
	NewNotes      string     `json:"new_notes,omitempty"`
	Reason        string     `json:"reason"`
	CorrectedByID string     `json:"corrected_by_user_id"`
	CorrectedAt   time.Time  `json:"corrected_at"`
	// EvidenceHash is the server-computed canonical sha256 over the
	// correction payload (correction-evidence package). Null for rows
	// created before migration 000014.
	EvidenceHash *string `json:"evidence_hash,omitempty"`
}

type RecordDetail struct {
	Record
	Corrections []Correction `json:"corrections"`
}

type HistoryFilter struct {
	From      string
	To        string
	StudentID string
	ClassID   string
	Status    string
	Limit     int32
	Offset    int32
}

type HistoryResult struct {
	Data   []HistoryRecord `json:"data"`
	Total  int64           `json:"total"`
	Limit  int32           `json:"limit"`
	Offset int32           `json:"offset"`
}

type HistoryRecord struct {
	ID               string     `json:"id"`
	Date             string     `json:"date"`
	Status           string     `json:"status"`
	CheckInAt        *time.Time `json:"check_in_at,omitempty"`
	Notes            string     `json:"notes,omitempty"`
	Source           string     `json:"source"`
	StudentID        string     `json:"student_id"`
	StudentNumber    string     `json:"student_number"`
	StudentName      string     `json:"student_name"`
	ClassID          string     `json:"class_id"`
	ClassName        string     `json:"class_name"`
	AcademicYearID   string     `json:"academic_year_id"`
	AcademicYearName string     `json:"academic_year_name"`
	Closed           bool       `json:"closed"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type SummaryFilter struct {
	From      string
	To        string
	StudentID string
	ClassID   string
}

type StudentSummary struct {
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

type ReportTotals struct {
	Students   int     `json:"students"`
	TotalDays  int32   `json:"total_days"`
	Present    int32   `json:"present"`
	Late       int32   `json:"late"`
	Sick       int32   `json:"sick"`
	Excused    int32   `json:"excused"`
	Absent     int32   `json:"absent"`
	Percentage float64 `json:"attendance_percentage"`
}

type SummaryResult struct {
	From   string           `json:"from"`
	To     string           `json:"to"`
	Data   []StudentSummary `json:"data"`
	Totals ReportTotals     `json:"totals"`
}

type ReportFilter struct {
	From      string
	To        string
	TermID    string
	GroupBy   string
	StudentID string
	ClassID   string
}

type ReportRow struct {
	PeriodStart string  `json:"period_start"`
	PeriodEnd   string  `json:"period_end"`
	ClassID     string  `json:"class_id"`
	ClassName   string  `json:"class_name"`
	TotalDays   int32   `json:"total_days"`
	Present     int32   `json:"present"`
	Late        int32   `json:"late"`
	Sick        int32   `json:"sick"`
	Excused     int32   `json:"excused"`
	Absent      int32   `json:"absent"`
	Percentage  float64 `json:"attendance_percentage"`
}

type ReportTotal struct {
	TotalDays  int32   `json:"total_days"`
	Present    int32   `json:"present"`
	Late       int32   `json:"late"`
	Sick       int32   `json:"sick"`
	Excused    int32   `json:"excused"`
	Absent     int32   `json:"absent"`
	Percentage float64 `json:"attendance_percentage"`
}

type ReportResult struct {
	From    string      `json:"from"`
	To      string      `json:"to"`
	GroupBy string      `json:"group_by"`
	Data    []ReportRow `json:"data"`
	Totals  ReportTotal `json:"totals"`
}
