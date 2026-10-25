package teacherpresence

import (
	"context"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

// GetGeofence exposes the current school location to any attendance reader.
func (s *Service) GetGeofence(ctx context.Context) (GeofenceConfig, error) {
	row, err := s.geofence(ctx)
	if err != nil {
		return GeofenceConfig{}, err
	}
	return geofenceFromRow(row), nil
}

// UpdateGeofence replaces the school location (admin only) and writes an
// audit entry, because the geofence is a sensitive configuration surface.
func (s *Service) UpdateGeofence(ctx context.Context, actor Actor, input GeofenceInput, requestID string) (GeofenceConfig, error) {
	if !hasRole(actor.Roles, "admin") {
		return GeofenceConfig{}, ErrForbidden
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return GeofenceConfig{}, ErrUnauthorized
	}
	if math.IsNaN(input.Latitude) || math.IsNaN(input.Longitude) ||
		input.Latitude < -90 || input.Latitude > 90 ||
		input.Longitude < -180 || input.Longitude > 180 {
		return GeofenceConfig{}, ErrInvalidInput
	}
	if input.RadiusMeters < 1 || input.RadiusMeters > 20000 {
		return GeofenceConfig{}, ErrInvalidInput
	}

	var updated db.SchoolGeofence
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		before, err := q.GetSchoolGeofence(ctx)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("get geofence before update: %w", err)
		}
		updated, err = q.UpdateSchoolGeofence(ctx, db.UpdateSchoolGeofenceParams{
			Latitude:     input.Latitude,
			Longitude:    input.Longitude,
			RadiusMeters: input.RadiusMeters,
			IsEnabled:    input.IsEnabled,
			UpdatedBy:    actorID,
		})
		if err != nil {
			return mapWriteError(fmt.Errorf("update school geofence: %w", err))
		}
		if err := s.auditData(ctx, q, actorID, "geofence.update", "school_geofence", pgtype.UUID{},
			geofenceFromRow(before), geofenceFromRow(updated), requestID); err != nil {
			return fmt.Errorf("write geofence audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return GeofenceConfig{}, err
	}
	return geofenceFromRow(updated), nil
}
