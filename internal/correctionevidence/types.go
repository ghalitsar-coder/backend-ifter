package correctionevidence

import (
	"errors"
	"time"
)

// Domain errors mapped to the shared error envelope by the HTTP handler.
var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrInvalidInput = errors.New("invalid input")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrFileTooLarge = errors.New("file too large")
)

// Actor is the authenticated subject performing an evidence operation.
type Actor struct {
	ID    string
	Roles []string
}

// EvidenceItem is the API shape of one evidence attachment (no binary and
// never the stored key). correction_id is nullable: evidence uploaded before
// any correction exists on the record stays unlinked.
type EvidenceItem struct {
	ID                 string    `json:"id"`
	AttendanceRecordID string    `json:"attendance_record_id"`
	CorrectionID       *string   `json:"correction_id"`
	FileName           string    `json:"file_name"`
	ContentType        string    `json:"content_type"`
	SizeBytes          int64     `json:"size_bytes"`
	Note               *string   `json:"note"`
	UploadedBy         string    `json:"uploaded_by"`
	CreatedAt          time.Time `json:"created_at"`
}

// UploadInput is one validated multipart evidence upload.
type UploadInput struct {
	Filename    string
	ContentType string
	Data        []byte
	Note        string
	// CorrectionID optionally pins the evidence to a specific correction of
	// the record; when empty the newest correction on the record is linked.
	CorrectionID string
}

// DocumentDownload is the private file served by the download endpoint.
type DocumentDownload struct {
	Filename    string
	ContentType string
	Data        []byte
}
