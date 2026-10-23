package imports

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/xuri/excelize/v2"

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

func parseDateCell(value string) (pgtype.Date, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return pgtype.Date{}, nil
	}
	layouts := []string{
		"2006-01-02", "2006-01-02 15:04:05", "02-01-2006", "02/01/2006",
		"2/1/2006", "02-01-06", "02/01/06", "2/1/06", "2006/01/02",
		"02 Jan 2006", "2 Jan 2006",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return pgtype.Date{Time: dateOnly(parsed), Valid: true}, nil
		}
	}
	serial, err := strconv.ParseFloat(value, 64)
	if err == nil {
		// Reject year-only numbers that would otherwise decode as 1900s serials.
		if serial < 15000 || serial > 100000 {
			return pgtype.Date{}, ErrInvalidInput
		}
		parsed, err := excelize.ExcelDateToTime(serial, false)
		if err == nil {
			return pgtype.Date{Time: dateOnly(parsed), Valid: true}, nil
		}
	}
	return pgtype.Date{}, ErrInvalidInput
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func todayDate() time.Time {
	return dateOnly(time.Now())
}

func dateString(value pgtype.Date) string {
	if !value.Valid {
		return ""
	}
	return value.Time.Format("2006-01-02")
}

func timePtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time
	return &t
}

func optionalText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}

func sameUUID(left, right pgtype.UUID) bool {
	return left.Valid == right.Valid && (!left.Valid || left.Bytes == right.Bytes)
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

func normalizeGender(value string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "":
		return "", nil
	case "L", "LAKI-LAKI", "LAKI LAKI", "MALE":
		return "L", nil
	case "P", "PEREMPUAN", "FEMALE":
		return "P", nil
	default:
		return "", ErrInvalidInput
	}
}

func normalizeStudentStatus(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return "", nil
	case "active", "aktif":
		return "active", nil
	case "graduated", "lulus":
		return "graduated", nil
	case "transferred", "pindah":
		return "transferred", nil
	case "inactive", "nonaktif", "non-aktif":
		return "inactive", nil
	default:
		return "", ErrInvalidInput
	}
}

func normalizeTeacherStatus(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return "", nil
	case "active", "aktif":
		return "active", nil
	case "inactive", "nonaktif", "non-aktif":
		return "inactive", nil
	default:
		return "", ErrInvalidInput
	}
}

func resolveDate(value string, start, end pgtype.Date) (pgtype.Date, error) {
	var chosen time.Time
	if strings.TrimSpace(value) == "" {
		chosen = todayDate()
		if chosen.Before(start.Time) {
			chosen = start.Time
		}
	} else {
		parsed, err := parseDateCell(value)
		if err != nil || !parsed.Valid {
			return pgtype.Date{}, ErrInvalidInput
		}
		chosen = parsed.Time
	}
	if chosen.Before(start.Time) || chosen.After(end.Time) {
		return pgtype.Date{}, ErrInvalidInput
	}
	return pgtype.Date{Time: chosen, Valid: true}, nil
}

func checkArchiveSize(data []byte, max uint64) error {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	var total uint64
	for _, file := range reader.File {
		total += file.UncompressedSize64
		if total > max {
			return ErrFileTooLarge
		}
	}
	return nil
}

func hashData(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func mapWriteError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, ErrInvalidInput),
		errors.Is(err, ErrNotFound),
		errors.Is(err, ErrConflict),
		errors.Is(err, ErrPreviewChanged):
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

func (s *Service) auditData(ctx context.Context, q *db.Queries, actorID pgtype.UUID, action string, entityID pgtype.UUID, after any, requestID string) error {
	payload, err := json.Marshal(after)
	if err != nil {
		return fmt.Errorf("encode import audit data: %w", err)
	}
	_, err = q.CreateAuditLog(ctx, db.CreateAuditLogParams{
		ActorUserID: actorID,
		Action:      action,
		EntityType:  "import_job",
		EntityID:    entityID,
		RequestID:   optionalText(requestID),
		AfterData:   string(payload),
	})
	return err
}

func jobFromDB(row db.ImportJob) Job {
	return Job{
		ID:               uuidString(row.ID),
		EntityType:       row.EntityType,
		ImportMode:       row.ImportMode,
		OriginalFilename: row.OriginalFilename,
		Status:           row.Status,
		TotalRows:        row.TotalRows,
		ValidRows:        row.ValidRows,
		DuplicateRows:    row.DuplicateRows,
		InvalidRows:      row.InvalidRows,
		SuccessRows:      row.SuccessRows,
		CreatedAt:        row.CreatedAt.Time,
		ValidatedAt:      timePtr(row.ValidatedAt),
		ConfirmedAt:      timePtr(row.ConfirmedAt),
		CompletedAt:      timePtr(row.CompletedAt),
	}
}

func rowErrorFromDB(row db.ImportRowError) RowError {
	return RowError{
		RowNumber: int(row.RowNumber),
		Column:    row.ColumnName.String,
		Code:      row.ErrorCode,
		Message:   row.Message,
	}
}

func columnMap(header []string, specs map[string][]string) (map[string]int, string) {
	normalized := make(map[string]int, len(header))
	for i, value := range header {
		name := strings.ToLower(strings.TrimSpace(value))
		if name == "" {
			continue
		}
		if _, exists := normalized[name]; !exists {
			normalized[name] = i
		}
	}
	columns := make(map[string]int, len(specs))
	for key, aliases := range specs {
		found := false
		for _, alias := range aliases {
			if index, ok := normalized[strings.ToLower(alias)]; ok {
				columns[key] = index
				found = true
				break
			}
		}
		if !found {
			return nil, key
		}
	}
	return columns, ""
}

func cell(row []string, columns map[string]int, key string) string {
	index, ok := columns[key]
	if !ok || index >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[index])
}

func preview(row int, action, key, name, className string, changed []string) PreviewRow {
	return PreviewRow{
		RowNumber:     row,
		Action:        action,
		Key:           key,
		Name:          name,
		ClassName:     className,
		ChangedFields: changed,
	}
}
