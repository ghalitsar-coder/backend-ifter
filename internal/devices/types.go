package devices

import (
	"errors"
	"time"
)

var (
	ErrUnauthorized = errors.New("device registry authorization required")
	ErrForbidden    = errors.New("insufficient permissions")
	ErrInvalidInput = errors.New("invalid device input")
	ErrNotFound     = errors.New("device not found")
	ErrConflict     = errors.New("device identifier already exists")
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
