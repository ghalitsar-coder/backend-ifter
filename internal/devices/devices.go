package devices

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

type normalized struct {
	name       string
	identifier string
	deviceType string
	location   pgtype.Text
	notes      pgtype.Text
}

func normalizeInput(in DeviceInput) (normalized, error) {
	name, err := cleanText(in.Name, maxNameRunes)
	if err != nil || name == "" {
		return normalized{}, ErrInvalidInput
	}
	identifier, err := cleanText(in.Identifier, maxIdentifierRunes)
	if err != nil || identifier == "" {
		return normalized{}, ErrInvalidInput
	}
	deviceType, err := cleanText(in.DeviceType, maxIdentifierRunes)
	if err != nil || !validType(deviceType) {
		return normalized{}, ErrInvalidInput
	}
	location, err := cleanText(in.Location, maxLocationRunes)
	if err != nil {
		return normalized{}, ErrInvalidInput
	}
	notes, err := cleanText(in.Notes, maxNotesRunes)
	if err != nil {
		return normalized{}, ErrInvalidInput
	}
	return normalized{
		name:       name,
		identifier: identifier,
		deviceType: deviceType,
		location:   optionalText(location),
		notes:      optionalText(notes),
	}, nil
}

// Create stores a new device (POST /devices); identifiers are unique
// case-insensitively, so a duplicate answers 409. Audited as device.create.
func (s *Service) Create(ctx context.Context, actor Actor, in DeviceInput, requestID string) (Device, error) {
	if err := requireAdmin(actor); err != nil {
		return Device{}, err
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return Device{}, ErrUnauthorized
	}
	n, err := normalizeInput(in)
	if err != nil {
		return Device{}, err
	}
	var created Device
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		row, err := q.CreateDevice(ctx, db.CreateDeviceParams{
			Name:       n.name,
			Identifier: n.identifier,
			DeviceType: n.deviceType,
			Location:   n.location,
			Notes:      n.notes,
		})
		if err != nil {
			return mapWriteError(fmt.Errorf("create device: %w", err))
		}
		created = deviceFromModel(row)
		return s.auditData(ctx, q, actorID, "device.create", row.ID, nil, newDeviceView(created), requestID)
	})
	if err != nil {
		return Device{}, err
	}
	return created, nil
}

// Update replaces a device's descriptive fields (PUT /devices/:id); status has
// its own PATCH surface. Audited as device.update with before/after snapshots.
func (s *Service) Update(ctx context.Context, actor Actor, idValue string, in DeviceInput, requestID string) (Device, error) {
	if err := requireAdmin(actor); err != nil {
		return Device{}, err
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return Device{}, ErrUnauthorized
	}
	id, err := parseUUID(idValue)
	if err != nil {
		return Device{}, ErrInvalidInput
	}
	n, err := normalizeInput(in)
	if err != nil {
		return Device{}, err
	}
	var updated Device
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		before, err := q.GetDeviceForUpdate(ctx, id)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("get device for update: %w", err)
		}
		row, err := q.UpdateDevice(ctx, db.UpdateDeviceParams{
			ID:         id,
			Name:       n.name,
			Identifier: n.identifier,
			DeviceType: n.deviceType,
			Location:   n.location,
			Notes:      n.notes,
		})
		if err != nil {
			return mapWriteError(fmt.Errorf("update device: %w", err))
		}
		updated = deviceFromModel(row)
		return s.auditData(ctx, q, actorID, "device.update", id, newDeviceView(deviceFromModel(before)), newDeviceView(updated), requestID)
	})
	if err != nil {
		return Device{}, err
	}
	return updated, nil
}

// SetStatus activates or deactivates a device (PATCH /devices/:id/status).
// Devices are retired this way instead of deleted. Audited as device.status.
func (s *Service) SetStatus(ctx context.Context, actor Actor, idValue, status, requestID string) (Device, error) {
	if err := requireAdmin(actor); err != nil {
		return Device{}, err
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return Device{}, ErrUnauthorized
	}
	id, err := parseUUID(idValue)
	if err != nil {
		return Device{}, ErrInvalidInput
	}
	status = strings.TrimSpace(status)
	if !validStatus(status) {
		return Device{}, ErrInvalidInput
	}
	var updated Device
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		before, err := q.GetDeviceForUpdate(ctx, id)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("get device for update: %w", err)
		}
		if before.Status == status {
			updated = deviceFromModel(before)
			return nil
		}
		row, err := q.UpdateDeviceStatus(ctx, db.UpdateDeviceStatusParams{ID: id, Status: status})
		if err != nil {
			return mapWriteError(fmt.Errorf("update device status: %w", err))
		}
		updated = deviceFromModel(row)
		return s.auditData(ctx, q, actorID, "device.status", id, newDeviceView(deviceFromModel(before)), newDeviceView(updated), requestID)
	})
	if err != nil {
		return Device{}, err
	}
	return updated, nil
}
