package masterdata

import (
	"errors"
	"time"
)

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrInvalidInput = errors.New("invalid input")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
)

type AcademicYear struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	StartDate string    `json:"start_date"`
	EndDate   string    `json:"end_date"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AcademicYearInput struct {
	Name      string
	StartDate string
	EndDate   string
	IsActive  bool
}

type Term struct {
	ID             string `json:"id"`
	AcademicYearID string `json:"academic_year_id"`
	TermNumber     int16  `json:"term_number"`
	Name           string `json:"name"`
	StartDate      string `json:"start_date"`
	EndDate        string `json:"end_date"`
}

type TermInput struct {
	TermNumber int16
	Name       string
	StartDate  string
	EndDate    string
}

type Class struct {
	ID                  string    `json:"id"`
	AcademicYearID      string    `json:"academic_year_id"`
	AcademicYearName    string    `json:"academic_year_name"`
	Name                string    `json:"name"`
	GradeLevel          int16     `json:"grade_level"`
	Major               string    `json:"major,omitempty"`
	Status              string    `json:"status"`
	HomeroomTeacherID   string    `json:"homeroom_teacher_id,omitempty"`
	HomeroomTeacherName string    `json:"homeroom_teacher_name,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type ClassDetail struct {
	Class
	Homeroom *HomeroomAssignment `json:"homeroom,omitempty"`
}

type ClassInput struct {
	AcademicYearID string
	Name           string
	GradeLevel     int16
	Major          string
}

type ClassUpdateInput struct {
	Name       string
	GradeLevel int16
	Major      string
}

type ClassFilter struct {
	AcademicYearID string
	Status         string
}

type HomeroomAssignment struct {
	ID          string    `json:"id"`
	TeacherID   string    `json:"teacher_id"`
	TeacherName string    `json:"teacher_name"`
	ClassID     string    `json:"class_id"`
	StartDate   string    `json:"start_date"`
	EndDate     string    `json:"end_date,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type HomeroomInput struct {
	TeacherID string
	StartDate string
}

type Teacher struct {
	ID             string    `json:"id"`
	UserID         string    `json:"user_id,omitempty"`
	EmployeeNumber string    `json:"employee_number,omitempty"`
	FullName       string    `json:"full_name"`
	Email          string    `json:"email,omitempty"`
	Phone          string    `json:"phone,omitempty"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type TeacherInput struct {
	EmployeeNumber string
	FullName       string
	Email          string
	Phone          string
	UserID         string
	Status         string
}

type TeacherUpdateInput struct {
	EmployeeNumber string
	FullName       string
	Email          string
	Phone          string
	UserID         string
}

type TeacherFilter struct {
	Search string
	Status string
}

type ClassRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Student struct {
	ID                    string    `json:"id"`
	StudentNumber         string    `json:"student_number"`
	NationalStudentNumber string    `json:"national_student_number,omitempty"`
	FullName              string    `json:"full_name"`
	Gender                string    `json:"gender,omitempty"`
	BirthDate             string    `json:"birth_date,omitempty"`
	AdmissionYear         int16     `json:"admission_year"`
	Status                string    `json:"status"`
	GuardianName          string    `json:"guardian_name,omitempty"`
	GuardianPhone         string    `json:"guardian_phone,omitempty"`
	CurrentClass          *ClassRef `json:"current_class,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type Enrollment struct {
	ID               string `json:"id"`
	ClassID          string `json:"class_id"`
	ClassName        string `json:"class_name"`
	AcademicYearID   string `json:"academic_year_id"`
	AcademicYearName string `json:"academic_year_name"`
	StartDate        string `json:"start_date"`
	EndDate          string `json:"end_date,omitempty"`
}

type StudentDetail struct {
	Student
	Enrollments []Enrollment `json:"enrollments"`
}

type ClassStudent struct {
	Student
	EnrollmentStart string `json:"enrollment_start"`
	EnrollmentEnd   string `json:"enrollment_end,omitempty"`
}

type StudentInput struct {
	StudentNumber         string
	NationalStudentNumber string
	FullName              string
	Gender                string
	BirthDate             string
	AdmissionYear         int16
	Status                string
	GuardianName          string
	GuardianPhone         string
	ClassID               string
	ClassStartDate        string
}

type StudentUpdateInput struct {
	StudentNumber         string
	NationalStudentNumber string
	FullName              string
	Gender                string
	BirthDate             string
	AdmissionYear         int16
	GuardianName          string
	GuardianPhone         string
}

type StudentClassInput struct {
	ClassID   string
	StartDate string
}

type StudentFilter struct {
	Search  string
	ClassID string
	Status  string
	SortBy  string
}

type Page struct {
	Limit  int32
	Offset int32
}
