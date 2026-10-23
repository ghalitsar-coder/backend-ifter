package masterdata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
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

func optionalUUID(value string) (pgtype.UUID, error) {
	if strings.TrimSpace(value) == "" {
		return pgtype.UUID{}, nil
	}
	return parseUUID(value)
}

func parseDate(value string) (pgtype.Date, error) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	if err != nil {
		return pgtype.Date{}, err
	}
	return pgtype.Date{Time: t, Valid: true}, nil
}

func optionalDate(value string) (pgtype.Date, error) {
	if strings.TrimSpace(value) == "" {
		return pgtype.Date{}, nil
	}
	return parseDate(value)
}

func dateString(value pgtype.Date) string {
	if !value.Valid {
		return ""
	}
	return value.Time.Format("2006-01-02")
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func todayDate() time.Time {
	return dateOnly(time.Now())
}

func optionalText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}

func timestamp(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}

func cleanText(value string, max int) (string, error) {
	value = strings.TrimSpace(value)
	if strings.IndexByte(value, 0) >= 0 || utf8.RuneCountInString(value) > max {
		return "", ErrInvalidInput
	}
	return value, nil
}

func requiredText(value string, max int) (string, error) {
	value, err := cleanText(value, max)
	if err != nil || value == "" {
		return "", ErrInvalidInput
	}
	return value, nil
}

func cleanEmail(value string) (string, error) {
	value, err := cleanText(value, 255)
	if err != nil || value == "" {
		return value, err
	}
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address != value {
		return "", ErrInvalidInput
	}
	return value, nil
}

func validStatus(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func normalizePage(page Page) Page {
	if page.Limit <= 0 {
		page.Limit = 50
	}
	if page.Limit > 100 {
		page.Limit = 100
	}
	if page.Offset < 0 {
		page.Offset = 0
	}
	return page
}

func resolveDate(value string, start, end pgtype.Date) (pgtype.Date, error) {
	var chosen time.Time
	if strings.TrimSpace(value) == "" {
		chosen = todayDate()
		if chosen.Before(start.Time) {
			chosen = start.Time
		}
	} else {
		parsed, err := parseDate(value)
		if err != nil {
			return pgtype.Date{}, ErrInvalidInput
		}
		chosen = parsed.Time
	}
	if chosen.Before(start.Time) || chosen.After(end.Time) {
		return pgtype.Date{}, ErrInvalidInput
	}
	return pgtype.Date{Time: chosen, Valid: true}, nil
}

func mapWriteError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, ErrUnauthorized),
		errors.Is(err, ErrInvalidInput),
		errors.Is(err, ErrNotFound),
		errors.Is(err, ErrConflict):
		return err
	case database.IsUniqueViolation(err), database.IsExclusionViolation(err):
		return ErrConflict
	case database.IsForeignKeyViolation(err),
		database.IsCheckViolation(err),
		database.IsNotNullViolation(err):
		return ErrInvalidInput
	default:
		return err
	}
}

func (s *Service) auditData(ctx context.Context, q *db.Queries, actorID pgtype.UUID, action, entityType string, entityID pgtype.UUID, before, after any, requestID string) error {
	var beforeData, afterData []byte
	var err error
	if before != nil {
		beforeData, err = json.Marshal(before)
		if err != nil {
			return fmt.Errorf("encode audit before data: %w", err)
		}
	}
	if after != nil {
		afterData, err = json.Marshal(after)
		if err != nil {
			return fmt.Errorf("encode audit after data: %w", err)
		}
	}
	_, err = q.CreateAuditLog(ctx, db.CreateAuditLogParams{
		ActorUserID: actorID,
		Action:      action,
		EntityType:  entityType,
		EntityID:    entityID,
		RequestID:   optionalText(requestID),
		BeforeData:  string(beforeData),
		AfterData:   string(afterData),
	})
	return err
}
