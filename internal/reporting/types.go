package reporting

import (
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrInvalidInput = errors.New("invalid input")
)

type Actor struct {
	ID    string
	Roles []string
}

type DashboardFilter struct {
	Date    string
	ClassID string
}

type Dashboard struct {
	Date               string              `json:"date"`
	Role               string              `json:"role"`
	Attendance         DashboardAttendance `json:"attendance"`
	Discipline         DashboardDiscipline `json:"discipline"`
	ActiveStudents     *int32              `json:"active_students,omitempty"`
	ActiveTeachers     *int32              `json:"active_teachers,omitempty"`
	Trend              []TrendPoint        `json:"trend"`
	LateStudents       []LateStudent       `json:"late_students"`
	UnrecordedStudents []UnrecordedStudent `json:"unrecorded_students"`
	RecentActivity     []AuditLog          `json:"recent_activity,omitempty"`
}

type DashboardAttendance struct {
	TotalStudents int32 `json:"total_students"`
	Recorded      int32 `json:"recorded"`
	Unrecorded    int32 `json:"unrecorded"`
	Present       int32 `json:"present"`
	Late          int32 `json:"late"`
	Sick          int32 `json:"sick"`
	Excused       int32 `json:"excused"`
	Absent        int32 `json:"absent"`
}

type DashboardDiscipline struct {
	RecordsToday int32 `json:"records_today"`
	RecordsMonth int32 `json:"records_month"`
	PointsMonth  int32 `json:"points_month"`
	NewRecords   int32 `json:"new_records"`
}

type TrendPoint struct {
	Date     string `json:"date"`
	Recorded int32  `json:"recorded"`
	Present  int32  `json:"present"`
	Late     int32  `json:"late"`
	Sick     int32  `json:"sick"`
	Excused  int32  `json:"excused"`
	Absent   int32  `json:"absent"`
}

type LateStudent struct {
	StudentID     string     `json:"student_id"`
	StudentNumber string     `json:"student_number"`
	StudentName   string     `json:"student_name"`
	ClassID       string     `json:"class_id"`
	ClassName     string     `json:"class_name"`
	CheckInAt     *time.Time `json:"check_in_at,omitempty"`
}

type UnrecordedStudent struct {
	StudentID     string `json:"student_id"`
	StudentNumber string `json:"student_number"`
	StudentName   string `json:"student_name"`
	ClassID       string `json:"class_id"`
	ClassName     string `json:"class_name"`
}

type AuditLogFilter struct {
	Action     string
	EntityType string
	EntityID   string
	ActorID    string
	From       string
	To         string
	Limit      int32
	Offset     int32
}

type AuditLog struct {
	ID          string          `json:"id"`
	ActorUserID string          `json:"actor_user_id,omitempty"`
	ActorName   string          `json:"actor_name"`
	Action      string          `json:"action"`
	EntityType  string          `json:"entity_type"`
	EntityID    string          `json:"entity_id,omitempty"`
	RequestID   string          `json:"request_id,omitempty"`
	BeforeData  json.RawMessage `json:"before_data,omitempty"`
	AfterData   json.RawMessage `json:"after_data,omitempty"`
	Reason      string          `json:"reason,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

type AuditLogResult struct {
	Data   []AuditLog `json:"data"`
	Total  int64      `json:"total"`
	Limit  int32      `json:"limit"`
	Offset int32      `json:"offset"`
}
