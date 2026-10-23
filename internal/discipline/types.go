package discipline

import (
	"errors"
	"time"
)

var (
	ErrUnauthorized = errors.New("discipline authorization required")
	ErrForbidden    = errors.New("insufficient permissions")
	ErrInvalidInput = errors.New("invalid discipline input")
	ErrNotFound     = errors.New("discipline data not found")
	ErrConflict     = errors.New("discipline data conflicts with an existing record")
)

type Actor struct {
	ID    string
	Roles []string
}

type Category struct {
	ID                     string    `json:"id"`
	Code                   string    `json:"code,omitempty"`
	Name                   string    `json:"name"`
	Description            string    `json:"description,omitempty"`
	DefaultPoints          *int      `json:"default_points,omitempty"`
	AutoOnAttendanceStatus string    `json:"auto_on_attendance_status,omitempty"`
	IsActive               bool      `json:"is_active"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type CategoryInput struct {
	Code                   string
	Name                   string
	Description            string
	DefaultPoints          *int
	AutoOnAttendanceStatus string
}

type Record struct {
	ID                 string    `json:"id"`
	StudentID          string    `json:"student_id"`
	StudentNumber      string    `json:"student_number"`
	StudentName        string    `json:"student_name"`
	ClassID            string    `json:"class_id"`
	ClassName          string    `json:"class_name"`
	CategoryID         string    `json:"category_id"`
	CategoryName       string    `json:"category_name"`
	OccurredAt         time.Time `json:"occurred_at"`
	OccurredDate       string    `json:"occurred_date"`
	Description        string    `json:"description"`
	Points             int32     `json:"points"`
	Status             string    `json:"status"`
	AttendanceRecordID *string   `json:"attendance_record_id,omitempty"`
	RecordedByUserID   string    `json:"recorded_by_user_id"`
	RecordedByName     string    `json:"recorded_by_name"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type RecordInput struct {
	StudentID   string
	CategoryID  string
	OccurredAt  string
	Description string
}

type RecordFilter struct {
	From       string
	To         string
	StudentID  string
	ClassID    string
	CategoryID string
	RecordedBy string
	Status     string
	Limit      int32
	Offset     int32
}

type RecordList struct {
	Data   []Record `json:"data"`
	Total  int64    `json:"total"`
	Limit  int32    `json:"limit"`
	Offset int32    `json:"offset"`
}

type Followup struct {
	ID            string    `json:"id"`
	RecordID      string    `json:"discipline_record_id"`
	Status        string    `json:"status"`
	Notes         string    `json:"notes,omitempty"`
	UpdatedByID   string    `json:"updated_by_user_id"`
	UpdatedByName string    `json:"updated_by_name"`
	CreatedAt     time.Time `json:"created_at"`
}

type RecordDetail struct {
	Record
	Followups []Followup `json:"followups"`
}

type FollowupInput struct {
	Status string
	Notes  string
}

type PointsInput struct {
	Points int32
	Reason string
}

type CategorySummary struct {
	CategoryID   string `json:"category_id"`
	CategoryName string `json:"category_name"`
	Count        int    `json:"count"`
	Points       int32  `json:"points"`
}

type StudentSummary struct {
	StudentID     string            `json:"student_id"`
	StudentNumber string            `json:"student_number"`
	StudentName   string            `json:"student_name"`
	TotalPoints   int32             `json:"total_points"`
	TotalRecords  int               `json:"total_records"`
	Categories    []CategorySummary `json:"categories"`
	Records       []Record          `json:"records"`
}

type ReportFilter struct {
	From       string
	To         string
	TermID     string
	StudentID  string
	ClassID    string
	CategoryID string
	RecordedBy string
	Status     string
}

type StatusSummary struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

type Report struct {
	From             string            `json:"from,omitempty"`
	To               string            `json:"to,omitempty"`
	TotalRecords     int               `json:"total_records"`
	CancelledRecords int               `json:"cancelled_records"`
	TotalPoints      int32             `json:"total_points"`
	ByCategory       []CategorySummary `json:"by_category"`
	ByStatus         []StatusSummary   `json:"by_status"`
	Data             []Record          `json:"data"`
}
