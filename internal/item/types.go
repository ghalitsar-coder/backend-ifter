package item

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound    = errors.New("item not found")
	ErrInvalidName = errors.New("invalid item name")
)

const (
	MaxNameLength  = 120
	DefaultPerPage = 20
	MaxPerPage     = 100
	MaxPage        = 100_000
)

// Item is the domain representation. It intentionally does not reuse the
// sqlc-generated row type, so the transport and service layers do not depend on
// database details.
type Item struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateInput struct {
	Name string
}

type ListInput struct {
	Page    int
	PerPage int
}

type ListResult struct {
	Items   []Item
	Page    int
	PerPage int
	Total   int64
}
