package sanctionrules

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

func int2Ptr(value pgtype.Int2) *int {
	if !value.Valid {
		return nil
	}
	converted := int(value.Int16)
	return &converted
}

func hasRole(roles []string, role string) bool {
	for _, candidate := range roles {
		if candidate == role {
			return true
		}
	}
	return false
}

// cleanText bounds a free-text field and rejects NUL bytes; empty stays empty.
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
// (docs/API_ADDENDUM.md §0: every sanction-rule change is audited).
func (s *Service) auditData(ctx context.Context, q *db.Queries, actorID pgtype.UUID, action string, entityID pgtype.UUID, before, after any, requestID string) error {
	var beforeData, afterData []byte
	var err error
	if before != nil {
		beforeData, err = json.Marshal(before)
		if err != nil {
			return fmt.Errorf("encode sanction rule audit before data: %w", err)
		}
	}
	if after != nil {
		afterData, err = json.Marshal(after)
		if err != nil {
			return fmt.Errorf("encode sanction rule audit after data: %w", err)
		}
	}
	_, err = q.CreateAuditLog(ctx, db.CreateAuditLogParams{
		ActorUserID: actorID,
		Action:      action,
		EntityType:  "sanction_rule",
		EntityID:    entityID,
		RequestID:   optionalText(requestID),
		BeforeData:  string(beforeData),
		AfterData:   string(afterData),
	})
	if err != nil {
		return fmt.Errorf("create sanction rule audit log: %w", err)
	}
	return nil
}

// ruleView is the snapshot stored in the audit row.
type ruleView struct {
	Name        string  `json:"name"`
	MinPoints   int     `json:"min_points"`
	MaxPoints   *int    `json:"max_points"`
	Action      string  `json:"action"`
	Level       *string `json:"level"`
	Description *string `json:"description"`
	IsActive    bool    `json:"is_active"`
}

func newRuleView(rule Rule) ruleView {
	return ruleView{
		Name:        rule.Name,
		MinPoints:   rule.MinPoints,
		MaxPoints:   rule.MaxPoints,
		Action:      rule.Action,
		Level:       rule.Level,
		Description: rule.Description,
		IsActive:    rule.IsActive,
	}
}

func ruleFromModel(row db.SanctionRule) Rule {
	return Rule{
		ID:          uuidString(row.ID),
		Name:        row.Name,
		MinPoints:   int(row.MinPoints),
		MaxPoints:   int2Ptr(row.MaxPoints),
		Action:      row.Action,
		Level:       textPtr(row.Level),
		Description: textPtr(row.Description),
		IsActive:    row.IsActive,
		CreatedAt:   timestamp(row.CreatedAt),
		UpdatedAt:   timestamp(row.UpdatedAt),
	}
}
