package attendance

import (
	"errors"
	"time"
)

var (
	ErrNotFound           = errors.New("attendance session not found")
	ErrNotStaff           = errors.New("token subject is not an active staff account")
	ErrForbidden          = errors.New("not allowed to record attendance for this class")
	ErrLocked             = errors.New("attendance session is locked")
	ErrInvalidClass       = errors.New("invalid class id")
	ErrInvalidSubject     = errors.New("invalid class subject id")
	ErrInvalidDate        = errors.New("invalid date")
	ErrInvalidStudent     = errors.New("invalid student id")
	ErrInvalidStatus      = errors.New("invalid attendance status")
	ErrEmptyAttendances   = errors.New("attendances must not be empty")
	ErrDuplicateStudent   = errors.New("duplicate student in attendances")
	ErrStudentNotEnrolled = errors.New("student is not actively enrolled in this class")
)

const dateLayout = "2006-01-02"

// Date is a calendar date without a time or zone. JSON is "YYYY-MM-DD".
type Date time.Time

func (d Date) MarshalJSON() ([]byte, error) {
	return []byte(`"` + time.Time(d).Format(dateLayout) + `"`), nil
}

func (d Date) Time() time.Time { return time.Time(d) }

func ParseDate(value string) (Date, error) {
	parsed, err := time.Parse(dateLayout, value)
	if err != nil {
		return Date{}, ErrInvalidDate
	}
	return Date(parsed), nil
}

// Class is school reference data used to pick the class an attendance session
// belongs to. WaliKelas is nil when the class has no homeroom teacher.
type Class struct {
	ID        int64   `json:"id"`
	NamaKelas string  `json:"nama_kelas"`
	Tingkat   int     `json:"tingkat"`
	WaliKelas *string `json:"wali_kelas"`
}

type Session struct {
	ID             int64     `json:"id"`
	ClassID        int64     `json:"class_id"`
	ClassSubjectID *int64    `json:"class_subject_id"`
	RecordedBy     int64     `json:"recorded_by"`
	Tanggal        Date      `json:"tanggal"`
	IsLocked       bool      `json:"is_locked"`
	CreatedAt      time.Time `json:"created_at"`
}

// Attendance is one student's row inside a session. Status is nil until the
// teacher records it.
type Attendance struct {
	StudentID  int64   `json:"student_id"`
	NIS        *string `json:"nis"`
	Nama       string  `json:"nama"`
	Status     *string `json:"status"`
	Keterangan *string `json:"keterangan"`
}

type SessionDetail struct {
	Session     Session      `json:"session"`
	Attendances []Attendance `json:"attendances"`
}

type CreateSessionInput struct {
	ClassID        int64
	ClassSubjectID *int64
	Tanggal        string
}

type RecordInput struct {
	SessionID   int64
	Attendances []AttendanceInput
}

type AttendanceInput struct {
	StudentID  int64   `json:"student_id"`
	Status     string  `json:"status"`
	Keterangan *string `json:"keterangan"`
}

// Actor is the authenticated caller. Manage is set for tu/admin, who may
// record for any class.
type Actor struct {
	UserID   int64
	SchoolID int64
	Manage   bool
}

func validStatus(status string) bool {
	switch status {
	case "hadir", "sakit", "izin", "alpa":
		return true
	}
	return false
}
