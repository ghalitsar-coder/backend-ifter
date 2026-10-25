package devices

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

const (
	maxNameRunes       = 100
	maxIdentifierRunes = 100
	maxLocationRunes   = 200
	maxNotesRunes      = 2000
)

// Service implements the device-registry use cases (F10). Every route is
// admin-only at the router; the service re-checks the role and audits every
// write, since the registry is the anchor for future tap data.
type Service struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	log     *slog.Logger
}

func NewService(pool *pgxpool.Pool, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		pool:    pool,
		queries: db.New(pool),
		log:     log,
	}
}

func requireAdmin(actor Actor) error {
	if !hasRole(actor.Roles, "admin") {
		return ErrForbidden
	}
	return nil
}

// List returns the registry ordered by name; filters are optional.
func (s *Service) List(ctx context.Context, actor Actor, filter DeviceFilter) (DeviceList, error) {
	if err := requireAdmin(actor); err != nil {
		return DeviceList{}, err
	}
	params := db.ListDevicesParams{}
	if filter.Status != "" {
		if !validStatus(filter.Status) {
			return DeviceList{}, ErrInvalidInput
		}
		params.Status = optionalText(filter.Status)
	}
	if filter.DeviceType != "" {
		if !validType(filter.DeviceType) {
			return DeviceList{}, ErrInvalidInput
		}
		params.DeviceType = optionalText(filter.DeviceType)
	}
	rows, err := s.queries.ListDevices(ctx, params)
	if err != nil {
		return DeviceList{}, fmt.Errorf("list devices: %w", err)
	}
	items := make([]Device, 0, len(rows))
	for _, row := range rows {
		items = append(items, deviceFromModel(row))
	}
	return DeviceList{Data: items}, nil
}

// Get returns one device for GET /devices/:id.
func (s *Service) Get(ctx context.Context, actor Actor, idValue string) (Device, error) {
	if err := requireAdmin(actor); err != nil {
		return Device{}, err
	}
	id, err := parseUUID(idValue)
	if err != nil {
		return Device{}, ErrInvalidInput
	}
	row, err := s.queries.GetDevice(ctx, id)
	if err != nil {
		if database.IsNotFound(err) {
			return Device{}, ErrNotFound
		}
		return Device{}, fmt.Errorf("get device: %w", err)
	}
	return deviceFromModel(row), nil
}
