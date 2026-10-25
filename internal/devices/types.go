package devices

import (
	"errors"
	"time"
)

var (
	ErrUnauthorized    = errors.New("device registry authorization required")
	ErrForbidden       = errors.New("insufficient permissions")
	ErrInvalidInput    = errors.New("invalid device input")
	ErrNotFound        = errors.New("device not found")
	ErrConflict        = errors.New("device identifier already exists")
	ErrInvalidToken    = errors.New("invalid device ingest token")
	ErrDeviceInactive  = errors.New("device is inactive")
	ErrStudentNotFound = errors.New("student not found")
)

// Ingest limits (F11 contract).
const (
	maxCardUIDRunes  = 64
	maxTapBatchSize  = 100
	tapFutureSkew    = 5 * time.Minute
	defaultTapLimit  = 50
	maxTapLimit      = 200
	ingestTokenBytes = 32
	ingestTokenPfx   = "dt_"
)

// Device types (F10 contract). The DB CHECK mirrors this set.
const (
	TypeRFIDReader = "rfid_reader"
	TypeGate       = "gate"
	TypeTablet     = "tablet"
	TypeOther      = "other"

	StatusActive   = "active"
	StatusInactive = "inactive"
)

func validType(value string) bool {
	switch value {
	case TypeRFIDReader, TypeGate, TypeTablet, TypeOther:
		return true
	default:
		return false
	}
}

func validStatus(value string) bool {
	return value == StatusActive || value == StatusInactive
}

// Actor is the authenticated principal performing a registry action. Routes
// are admin-only; the service re-checks the role (defense in depth).
type Actor struct {
	ID    string
	Roles []string
}

// Device is one registry row in the external shape of FITUR-FRONTEND.md / F10.
type Device struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Identifier string     `json:"identifier"`
	DeviceType string     `json:"device_type"`
	Location   *string    `json:"location"`
	Notes      *string    `json:"notes"`
	Status     string     `json:"status"`
	LastSeenAt *time.Time `json:"last_seen_at"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type DeviceInput struct {
	Name       string
	Identifier string
	DeviceType string
	Location   string
	Notes      string
}

type DeviceFilter struct {
	Status     string
	DeviceType string
}

type DeviceList struct {
	Data []Device `json:"data"`
}

// TapInput is one reader event. OccurredAt is reader time (RFC3339), which may
// arrive late from a buffered offline reader; only future dates are rejected.
type TapInput struct {
	CardUID    string    `json:"card_uid"`
	OccurredAt time.Time `json:"occurred_at"`
}

// TapBatch is the reader payload for POST /device-taps. Identifier must match
// the device the token belongs to, so a token copied to another reader fails.
type TapBatch struct {
	Identifier string     `json:"device_identifier"`
	Taps       []TapInput `json:"taps"`
}

// TapIngestResult answers an ingest call. Unknown counts accepted taps whose
// card UID is not mapped to a student yet; they are still logged with a NULL
// student so staff can trace the card. Mapping applies to later taps only.
type TapIngestResult struct {
	Accepted   int `json:"accepted"`
	Duplicates int `json:"duplicates"`
	Unknown    int `json:"unknown_cards"`
}

// Tap is one tap-log row in the staff-facing list.
type Tap struct {
	ID               string    `json:"id"`
	DeviceID         string    `json:"device_id"`
	DeviceName       string    `json:"device_name"`
	DeviceIdentifier string    `json:"device_identifier"`
	StudentID        *string   `json:"student_id"`
	StudentNumber    *string   `json:"student_number"`
	StudentName      *string   `json:"student_name"`
	CardUID          string    `json:"card_uid"`
	OccurredAt       time.Time `json:"occurred_at"`
	CreatedAt        time.Time `json:"created_at"`
}

type TapFilter struct {
	DeviceID  string
	StudentID string
	CardUID   string
	From      string
	To        string
	Limit     int32
	Offset    int32
}

type TapList struct {
	Data   []Tap `json:"data"`
	Total  int64 `json:"total"`
	Limit  int32 `json:"limit"`
	Offset int32 `json:"offset"`
}

// StudentCard is the card-to-student mapping surface. A nil CardUID means the
// student has no card.
type StudentCard struct {
	StudentID string  `json:"student_id"`
	CardUID   *string `json:"card_uid"`
}
