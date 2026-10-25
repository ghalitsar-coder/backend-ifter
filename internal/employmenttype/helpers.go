package employmenttype

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
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

// codePattern mirrors the employment_types_code_format CHECK of migration 000013.
var codePattern = regexp.MustCompile(`^[a-z0-9_]{1,30}$`)

// cleanText bounds a free-text field and rejects NUL bytes; empty stays empty.
func cleanText(value string, maxRunes int) (string, error) {
	value = strings.TrimSpace(value)
	if strings.IndexByte(value, 0) >= 0 || utf8.RuneCountInString(value) > maxRunes {
		return "", ErrInvalidInput
	}
	return value, nil
}

// normalizeInput validates a POST/PUT payload into trimmed fields.
func normalizeInput(in TypeInput) (code, name, description string, err error) {
	code, err = cleanText(in.Code, maxCodeRunes)
	if err != nil || code == "" || !codePattern.MatchString(code) {
		return "", "", "", ErrInvalidInput
	}
	name, err = cleanText(in.Name, maxNameRunes)
	if err != nil || name == "" {
		return "", "", "", ErrInvalidInput
	}
	description, err = cleanText(in.Description, maxDescriptionRunes)
	if err != nil {
		return "", "", "", ErrInvalidInput
	}
	return code, name, description, nil
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

// auditData writes an audit_logs row inside the caller's transaction
// (docs/API_ADDENDUM.md §0: every employment-type write is audited).
func (s *Service) auditData(ctx context.Context, q *db.Queries, actorID pgtype.UUID, action string, entityID pgtype.UUID, before, after any, requestID string) error {
	var beforeData, afterData []byte
	var err error
	if before != nil {
		beforeData, err = json.Marshal(before)
		if err != nil {
			return fmt.Errorf("encode employment type audit before data: %w", err)
		}
	}
	if after != nil {
		afterData, err = json.Marshal(after)
		if err != nil {
			return fmt.Errorf("encode employment type audit after data: %w", err)
		}
	}
	_, err = q.CreateAuditLog(ctx, db.CreateAuditLogParams{
		ActorUserID: actorID,
		Action:      action,
		EntityType:  "employment_type",
		EntityID:    entityID,
		RequestID:   optionalText(requestID),
		BeforeData:  string(beforeData),
		AfterData:   string(afterData),
	})
	if err != nil {
		return fmt.Errorf("create employment type audit log: %w", err)
	}
	return nil
}

func typeFromModel(row db.EmploymentType) EmploymentType {
	return EmploymentType{
		ID:          uuidString(row.ID),
		Code:        row.Code,
		Name:        row.Name,
		Description: textPtr(row.Description),
		IsActive:    row.IsActive,
		CreatedAt:   timestamp(row.CreatedAt),
		UpdatedAt:   timestamp(row.UpdatedAt),
	}
}
