package imports

import (
	"errors"
	"time"
)

var (
	ErrInvalidInput   = errors.New("invalid input")
	ErrNotFound       = errors.New("not found")
	ErrConflict       = errors.New("conflict")
	ErrPreviewChanged = errors.New("preview changed")
	ErrFileTooLarge   = errors.New("file too large")
)

type Config struct {
	Dir            string
	MaxFileSize    int64
	MaxRows        int
	FileRetention  time.Duration
	ErrorRetention time.Duration
}

type Entity string

const (
	EntityStudents Entity = "students"
	EntityTeachers Entity = "teachers"
)

type Mode string

const (
	ModeCreateOnly Mode = "create_only"
	ModeUpsert     Mode = "upsert"
)

type Job struct {
	ID               string     `json:"id"`
	EntityType       string     `json:"entity_type"`
	ImportMode       string     `json:"import_mode"`
	OriginalFilename string     `json:"original_filename"`
	Status           string     `json:"status"`
	TotalRows        int32      `json:"total_rows"`
	ValidRows        int32      `json:"valid_rows"`
	DuplicateRows    int32      `json:"duplicate_rows"`
	InvalidRows      int32      `json:"invalid_rows"`
	SuccessRows      int32      `json:"success_rows"`
	CreatedAt        time.Time  `json:"created_at"`
	ValidatedAt      *time.Time `json:"validated_at,omitempty"`
	ConfirmedAt      *time.Time `json:"confirmed_at,omitempty"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
}

type RowError struct {
	RowNumber int    `json:"row_number"`
	Column    string `json:"column,omitempty"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

type PreviewRow struct {
	RowNumber     int      `json:"row_number"`
	Action        string   `json:"action"`
	Key           string   `json:"key"`
	Name          string   `json:"name"`
	ClassName     string   `json:"class_name,omitempty"`
	ChangedFields []string `json:"changed_fields,omitempty"`
}

type Validation struct {
	TotalRows     int
	ValidRows     int
	DuplicateRows int
	InvalidRows   int
	Rows          []PreviewRow
	Errors        []RowError

	studentRows []studentImportRow
	teacherRows []teacherImportRow
}

type UploadResult struct {
	Job     Job          `json:"job"`
	Preview []PreviewRow `json:"preview"`
	Errors  []RowError   `json:"errors"`
}

type Detail struct {
	Job     Job          `json:"job"`
	Preview []PreviewRow `json:"preview,omitempty"`
	Errors  []RowError   `json:"errors"`
}

type ListFilter struct {
	EntityType string
	Status     string
	Limit      int32
	Offset     int32
}

type ExportFilter struct {
	Status  string
	ClassID string
	Search  string
}
