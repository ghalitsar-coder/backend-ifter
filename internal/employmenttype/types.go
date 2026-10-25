// Package employmenttype implements the employment-type master-data feature
// package (docs/API_ADDENDUM.md §6.5 + package spec, migration 000013): the
// code-keyed list of teacher employment statuses (seeded: permanent,
// honorary, staff_tu) referenced by teachers.employment_type_id.
//
// Layering follows the existing modular monolith (handler -> usecase ->
// repository): this package is the usecase/service layer, persistence goes
// through sqlc-generated queries in internal/db. The surface is master data,
// so route reachability is gated by groups.MasterData (admin + staff_tu) at
// the router; like the other masterdata services this layer does not re-check
// role codes, it only requires an authenticated actor for the audit trail.
package employmenttype

import (
	"errors"
	"time"
)

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrInvalidInput = errors.New("invalid input")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("employment type code already exists")
)

// Field bounds mirror the CHECK constraints of migration 000013.
const (
	maxCodeRunes        = 30
	maxNameRunes        = 100
	maxDescriptionRunes = 2000
)

// Actor is the authenticated principal performing an employment-type action.
type Actor struct {
	ID    string
	Roles []string
}

// EmploymentType is one row in the API shape of docs/API_ADDENDUM.md §6.5.
type EmploymentType struct {
	ID          string    `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TypeList is the unpaginated list envelope { "data": [...] }.
type TypeList struct {
	Data []EmploymentType `json:"data"`
}

// TypeEnvelope is the single-resource shape { "employment_type": {...} }.
type TypeEnvelope struct {
	Type EmploymentType `json:"employment_type"`
}

// TypeInput is one POST/PUT payload (already decoded from snake_case JSON).
// IsActive is a pointer so an omitted field keeps the row's current value on
// PUT (and defaults to true on POST).
type TypeInput struct {
	Code        string
	Name        string
	Description string
	IsActive    *bool
}
