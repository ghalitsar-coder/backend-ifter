package devices

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

func parseUUID(value string) (pgtype.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: id, Valid: true}, nil
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}

func timestamp(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}

func timestampPtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time
	return &t
}

func textPtr(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	text := value.String
	return &text
}

func optionalText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}

func hasRole(roles []string, role string) bool {
	for _, candidate := range roles {
		if candidate == role {
			return true
		}
	}
	return false
}

func cleanText(value string, maxRunes int) (string, error) {
	value = strings.TrimSpace(value)
	if strings.IndexByte(value, 0) >= 0 || utf8.RuneCountInString(value) > maxRunes {
		return "", ErrInvalidInput
	}
	return value, nil
}

func mapWriteError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, ErrUnauthorized),
		errors.Is(err, ErrForbidden),
		errors.Is(err, ErrInvalidInput),
		errors.Is(err, ErrNotFound),
		errors.Is(err, ErrConflict):
		return err
	case database.IsUniqueViolation(err):
		return ErrConflict
	case database.IsForeignKeyViolation(err),
		database.IsCheckViolation(err),
		database.IsNotNullViolation(err):
		return ErrInvalidInput
	default:
		return err
	}
}

// deviceFromModel maps a sqlc row to the API shape; blank optional columns
// become JSON null.
func deviceFromModel(row db.Device) Device {
	return Device{
		ID:         uuidString(row.ID),
		Name:       row.Name,
		Identifier: row.Identifier,
		DeviceType: row.DeviceType,
		Location:   textPtr(row.Location),
		Notes:      textPtr(row.Notes),
		Status:     row.Status,
		LastSeenAt: timestampPtr(row.LastSeenAt),
		CreatedAt:  timestamp(row.CreatedAt),
		UpdatedAt:  timestamp(row.UpdatedAt),
	}
}

type deviceView struct {
	Name       string  `json:"name"`
	Identifier string  `json:"identifier"`
	DeviceType string  `json:"device_type"`
	Location   *string `json:"location"`
	Notes      *string `json:"notes"`
	Status     string  `json:"status"`
}

func newDeviceView(device Device) deviceView {
	return deviceView{
		Name:       device.Name,
		Identifier: device.Identifier,
		DeviceType: device.DeviceType,
		Location:   device.Location,
		Notes:      device.Notes,
		Status:     device.Status,
	}
}

func (s *Service) auditData(ctx context.Context, q *db.Queries, actorID pgtype.UUID, action string, entityID pgtype.UUID, before, after any, requestID string) error {
	var beforeData, afterData []byte
	var err error
	if before != nil {
		beforeData, err = json.Marshal(before)
		if err != nil {
			return fmt.Errorf("encode device audit before data: %w", err)
		}
	}
	if after != nil {
		afterData, err = json.Marshal(after)
		if err != nil {
			return fmt.Errorf("encode device audit after data: %w", err)
		}
	}
	_, err = q.CreateAuditLog(ctx, db.CreateAuditLogParams{
		ActorUserID: actorID,
		Action:      action,
		EntityType:  "device",
		EntityID:    entityID,
		RequestID:   optionalText(requestID),
		BeforeData:  string(beforeData),
		AfterData:   string(afterData),
	})
	if err != nil {
		return fmt.Errorf("create device audit log: %w", err)
	}
	return nil
}
