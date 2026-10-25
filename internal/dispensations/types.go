package dispensations

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
	// ErrFileTooLarge is returned when an uploaded document exceeds the
	// shared imports max_file_size limit.
	ErrFileTooLarge = errors.New("file too large")
	// ErrAccountNotLinked means the signed-in user has no linked student
	// record (students.user_id, migration 000015). /my/* is unavailable
	// until the student-account package provisions the link.
	ErrAccountNotLinked = errors.New("student account not linked")
)

// Enum domains (docs/API_ADDENDUM.md §6.2). The external shape lives here.
const (
	TypeSick   = "sick"
	TypeLate   = "late"
	TypePermit = "permit"
	TypeOther  = "other"

	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusRejected = "rejected"
)

func validType(value string) bool {
	switch value {
	case TypeSick, TypeLate, TypePermit, TypeOther:
		return true
	default:
		return false
	}
}

func validStatus(value string) bool {
	switch value {
	case StatusPending, StatusApproved, StatusRejected:
		return true
	default:
		return false
	}
}

func validDecision(value string) bool {
	return value == StatusApproved || value == StatusRejected
}

// Actor is the authenticated principal performing a dispensation action.
type Actor struct {
	ID    string
	Roles []string
}

// DocumentInput is an optional upload attached to a create request. The
// handler reads the multipart part into memory (bounded by the shared
// imports max_file_size) and the service persists it to the private store.
type DocumentInput struct {
	Filename    string
	ContentType string
	Data        []byte
}

// CreateInput is the service payload for both staff and student creates.
// StudentID is empty for /my/dispensations (the service resolves the actor's
// own student from students.user_id).
type CreateInput struct {
	StudentID string
	Type      string
	Reason    string
	StartDate string
	EndDate   string
	Document  *DocumentInput
}

// VerifyInput is the POST /dispensations/:id/verify payload.
type VerifyInput struct {
	Status string
	Note   string
}

// ListFilter holds validated query parameters for the list endpoints.
type ListFilter struct {
	Status    string
	Type      string
	StudentID string
	ClassID   string
	From      string
	To        string
	Limit     int32
	Offset    int32
}

// Item is one dispensation row in the API shape of docs/API_ADDENDUM.md §6.2.
type Item struct {
	ID                string     `json:"id"`
	StudentID         string     `json:"student_id"`
	StudentName       string     `json:"student_name"`
	ClassID           string     `json:"class_id"`
	ClassName         string     `json:"class_name"`
	Type              string     `json:"type"`
	Reason            string     `json:"reason"`
	StartDate         string     `json:"start_date"`
	EndDate           *string    `json:"end_date"`
	Status            string     `json:"status"`
	DocumentAvailable bool       `json:"document_available"`
	SubmittedBy       string     `json:"submitted_by"`
	VerifiedBy        *string    `json:"verified_by"`
	VerifiedAt        *time.Time `json:"verified_at"`
	Note              *string    `json:"note"`
	CreatedAt         time.Time  `json:"created_at"`
}

// ListResult is the paginated list envelope.
type ListResult struct {
	Data   []Item `json:"data"`
	Total  int64  `json:"total"`
	Limit  int32  `json:"limit"`
	Offset int32  `json:"offset"`
}

// DocumentDownload is the stored document returned by GET /dispensations/:id/document.
type DocumentDownload struct {
	Filename    string
	ContentType string
	Data        []byte
}
