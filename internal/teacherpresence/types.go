package teacherpresence

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

// Actor is the authenticated principal performing a presence action.
type Actor struct {
	ID    string
	Roles []string
}

// GeofenceConfig is the school location used for presence geofencing.
type GeofenceConfig struct {
	Latitude     float64 `json:"latitude"`
	Longitude    float64 `json:"longitude"`
	RadiusMeters int32   `json:"radius_meters"`
	IsEnabled    bool    `json:"is_enabled"`
}

// GeofenceInput is the PUT /geofence payload (full replace).
type GeofenceInput struct {
	Latitude     float64
	Longitude    float64
	RadiusMeters int32
	IsEnabled    bool
}

// CheckInput carries the client-reported GPS fix for a check-in or check-out.
// The server computes distance and geofence verdict from these values.
type CheckInput struct {
	Latitude  float64
	Longitude float64
	Accuracy  float64
	CheckTime string // optional RFC3339; server-now when empty
	Notes     string
}

// CheckInResult is the POST /teacher-presence/check-in response body.
type CheckInResult struct {
	ID             string    `json:"id"`
	TeacherID      string    `json:"teacher_id"`
	PresenceDate   string    `json:"presence_date"`
	CheckInAt      time.Time `json:"check_in_at"`
	WithinGeofence bool      `json:"within_geofence"`
	DistanceMeters float64   `json:"distance_meters"`
	Message        string    `json:"message"`
}

// CheckOutResult is the POST /teacher-presence/check-out response body.
type CheckOutResult struct {
	ID                     string    `json:"id"`
	TeacherID              string    `json:"teacher_id"`
	PresenceDate           string    `json:"presence_date"`
	CheckInAt              time.Time `json:"check_in_at"`
	CheckOutAt             time.Time `json:"check_out_at"`
	DurationMinutes        float64   `json:"duration_minutes"`
	CheckOutWithinGeofence bool      `json:"check_out_within_geofence"`
	CheckOutDistanceMeters float64   `json:"check_out_distance_meters"`
	Message                string    `json:"message"`
}

// TodayResult is the GET /teacher-presence/today response body. Null fields
// mean the teacher has not checked in yet today.
type TodayResult struct {
	TeacherID      string     `json:"teacher_id"`
	PresenceDate   string     `json:"presence_date"`
	CheckInAt      *time.Time `json:"check_in_at"`
	CheckOutAt     *time.Time `json:"check_out_at"`
	WithinGeofence *bool      `json:"within_geofence"`
	DistanceMeters *float64   `json:"distance_meters"`
}

// HistoryEntry is one paginated row of GET /teacher-presence/history.
type HistoryEntry struct {
	ID              string     `json:"id"`
	TeacherID       string     `json:"teacher_id"`
	PresenceDate    string     `json:"presence_date"`
	CheckInAt       time.Time  `json:"check_in_at"`
	CheckOutAt      *time.Time `json:"check_out_at"`
	DurationMinutes float64    `json:"duration_minutes"`
	WithinGeofence  bool       `json:"within_geofence"`
	DistanceMeters  float64    `json:"distance_meters"`
}

// HistoryResult is the paginated list envelope for the history endpoint.
type HistoryResult struct {
	Data   []HistoryEntry `json:"data"`
	Total  int64          `json:"total"`
	Limit  int32          `json:"limit"`
	Offset int32          `json:"offset"`
}

// HistoryFilter holds validated query parameters for the history list.
type HistoryFilter struct {
	From   string
	To     string
	Limit  int32
	Offset int32
}
