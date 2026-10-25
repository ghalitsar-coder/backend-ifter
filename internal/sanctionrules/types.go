// Package sanctionrules implements the sanction-rules feature package
// (docs/API_ADDENDUM.md §6.4 + package spec, migration 000012): the point
// thresholds that map a student's accumulated discipline points to a sanction
// tier (level + action).
//
// Layering follows the existing modular monolith (handler -> usecase ->
// repository): this package is the usecase/service layer, persistence goes
// through sqlc-generated queries in internal/db. Overlapping ranges are
// rejected here, in the use case — not by a DB constraint (package spec).
package sanctionrules

import (
	"errors"
	"time"
)

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrInvalidInput = errors.New("invalid input")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("sanction rule ranges overlap")
)

// Field bounds mirror the CHECK constraints of migration 000012.
const (
	maxNameRunes        = 100
	maxActionRunes      = 100
	maxLevelRunes       = 30
	maxDescriptionRunes = 2000
	// maxPoints mirrors the smallint column domain.
	maxPoints = 32767
	// openEndedMax mirrors the CHECK sentinel for a NULL max_points
	// (COALESCE(max_points, 1000000000)): the range is open-ended upward.
	openEndedMax = 1000000000
)

// Actor is the authenticated principal performing a sanction-rule action.
type Actor struct {
	ID    string
	Roles []string
}

// Rule is one sanction rule in the API shape of docs/API_ADDENDUM.md §6.4
// (superset: level carries the tier label surfaced as current_level on
// GET /discipline/students/:id).
type Rule struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	MinPoints   int       `json:"min_points"`
	MaxPoints   *int      `json:"max_points"`
	Action      string    `json:"action"`
	Level       *string   `json:"level"`
	Description *string   `json:"description"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// RuleList is the unpaginated list envelope { "data": [...] }.
type RuleList struct {
	Data []Rule `json:"data"`
}

// RuleEnvelope is the single-resource shape { "sanction_rule": {...} }.
type RuleEnvelope struct {
	Rule Rule `json:"sanction_rule"`
}

// RuleInput is one POST/PUT/replace-all rule payload (already decoded from
// snake_case JSON). MinPoints is a pointer so an omitted field is rejected
// rather than silently read as 0. IsActive is only honored by the replace-all
// surface; single create keeps the row active and PATCH :id/status flips it.
type RuleInput struct {
	Name        string
	MinPoints   *int
	MaxPoints   *int
	Action      string
	Level       string
	Description string
	IsActive    *bool
}
