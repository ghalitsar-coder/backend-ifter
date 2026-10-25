package dispensations

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

func parseDate(value string) (pgtype.Date, error) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	if err != nil {
		return pgtype.Date{}, err
	}
	return pgtype.Date{Time: t, Valid: true}, nil
}

func dateString(value pgtype.Date) *string {
	if !value.Valid {
		return nil
	}
	text := value.Time.Format("2006-01-02")
	return &text
}

func dateValue(value pgtype.Date) string {
	if !value.Valid {
		return ""
	}
	return value.Time.Format("2006-01-02")
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

// cleanText bounds a free-text field and rejects NUL bytes; empty stays empty.
func cleanText(value string, maxRunes int) (string, error) {
	value = strings.TrimSpace(value)
	if strings.IndexByte(value, 0) >= 0 || utf8.RuneCountInString(value) > maxRunes {
		return "", ErrInvalidInput
	}
	return value, nil
}

func normalizePage(limit, offset int32) (int32, int32) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
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
		errors.Is(err, ErrConflict),
		errors.Is(err, ErrFileTooLarge),
		errors.Is(err, ErrAccountNotLinked):
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

func itemFromGetRow(row db.GetDispensationRow) Item {
	return Item{
		ID:                uuidString(row.ID),
		StudentID:         uuidString(row.StudentID),
		StudentName:       row.StudentName,
		ClassID:           uuidString(row.ClassID),
		ClassName:         row.ClassName,
		Type:              row.Type,
		Reason:            row.Reason,
		StartDate:         dateValue(row.StartDate),
		EndDate:           dateString(row.EndDate),
		Status:            row.Status,
		DocumentAvailable: row.DocumentStoredKey.Valid,
		SubmittedBy:       uuidString(row.SubmittedBy),
		VerifiedBy:        nullableUUIDString(row.VerifiedBy),
		VerifiedAt:        timestampPtr(row.VerifiedAt),
		Note:              textPtr(row.Note),
		CreatedAt:         timestamp(row.CreatedAt),
	}
}

func itemFromListRow(row db.ListDispensationsRow) Item {
	return Item{
		ID:                uuidString(row.ID),
		StudentID:         uuidString(row.StudentID),
		StudentName:       row.StudentName,
		ClassID:           uuidString(row.ClassID),
		ClassName:         row.ClassName,
		Type:              row.Type,
		Reason:            row.Reason,
		StartDate:         dateValue(row.StartDate),
		EndDate:           dateString(row.EndDate),
		Status:            row.Status,
		DocumentAvailable: row.DocumentStoredKey.Valid,
		SubmittedBy:       uuidString(row.SubmittedBy),
		VerifiedBy:        nullableUUIDString(row.VerifiedBy),
		VerifiedAt:        timestampPtr(row.VerifiedAt),
		Note:              textPtr(row.Note),
		CreatedAt:         timestamp(row.CreatedAt),
	}
}

func nullableUUIDString(value pgtype.UUID) *string {
	if !value.Valid {
		return nil
	}
	text := uuidString(value)
	return &text
}

// verificationView is the audited subset of a dispensation row.
func verificationView(status string, verifiedBy pgtype.UUID, verifiedAt pgtype.Timestamptz, note pgtype.Text) map[string]any {
	return map[string]any{
		"status":      status,
		"verified_by": nullableUUIDString(verifiedBy),
		"verified_at": timestampPtr(verifiedAt),
		"note":        textPtr(note),
	}
}

func (s *Service) auditData(ctx context.Context, q *db.Queries, actorID pgtype.UUID, action, entityType string, entityID pgtype.UUID, before, after any, requestID string) error {
	var beforeData, afterData []byte
	var err error
	if before != nil {
		beforeData, err = json.Marshal(before)
		if err != nil {
			return fmt.Errorf("encode dispensation audit before data: %w", err)
		}
	}
	if after != nil {
		afterData, err = json.Marshal(after)
		if err != nil {
			return fmt.Errorf("encode dispensation audit after data: %w", err)
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

// allowedDocumentExtensions maps the file extension of an uploaded document
// to its served content type. Anything else (docx, svg, html, ...) is
// rejected — the store only ever holds pdf/jpeg/png.
var allowedDocumentExtensions = map[string]string{
	".pdf":  "application/pdf",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
}

func extensionOf(key string) string {
	return strings.ToLower(filepath.Ext(key))
}

func contentTypeForExtension(extension string) string {
	if contentType, ok := allowedDocumentExtensions[extension]; ok {
		return contentType
	}
	return "application/octet-stream"
}

// validateDocument checks extension, declared MIME (when present) and the
// magic bytes of the payload against each other before anything is stored.
func validateDocument(filename, declaredContentType string, data []byte) (string, error) {
	extension := strings.ToLower(filepath.Ext(strings.ReplaceAll(strings.TrimSpace(filename), "\\", "/")))
	expectedContentType, ok := allowedDocumentExtensions[extension]
	if !ok {
		return "", ErrInvalidInput
	}
	if declaredContentType != "" {
		declared := strings.ToLower(strings.TrimSpace(strings.Split(declaredContentType, ";")[0]))
		if declared != expectedContentType {
			return "", ErrInvalidInput
		}
	}
	if !matchesMagicBytes(extension, data) {
		return "", ErrInvalidInput
	}
	return extension, nil
}

func matchesMagicBytes(extension string, data []byte) bool {
	switch extension {
	case ".pdf":
		return bytes.HasPrefix(data, []byte("%PDF"))
	case ".jpg", ".jpeg":
		return len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF
	case ".png":
		return len(data) >= 4 && bytes.Equal(data[:4], []byte{0x89, 'P', 'N', 'G'})
	default:
		return false
	}
}

// storage file helpers, mirroring the imports service: private dir 0700,
// files 0600, keys reduced to their base name so a path can never escape the
// storage directory.
func (s *Service) ensureDir() error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create dispensation storage directory: %w", err)
	}
	return nil
}

func (s *Service) writeFile(key string, data []byte) error {
	key = filepath.Base(key)
	if key == "." || key == ".." || key == "" {
		return ErrInvalidInput
	}
	return os.WriteFile(filepath.Join(s.dir, key), data, 0o600)
}

func (s *Service) readFile(key string) ([]byte, error) {
	key = filepath.Base(key)
	if key == "." || key == ".." || key == "" {
		return nil, ErrNotFound
	}
	return os.ReadFile(filepath.Join(s.dir, key))
}

func (s *Service) removeFile(key string) error {
	key = filepath.Base(key)
	if key == "." || key == ".." || key == "" {
		return nil
	}
	err := os.Remove(filepath.Join(s.dir, key))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
