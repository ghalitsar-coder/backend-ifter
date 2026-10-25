package studentaccounts

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

func uuidPtr(value pgtype.UUID) *string {
	if !value.Valid {
		return nil
	}
	text := uuidString(value)
	return &text
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
	moment := value.Time
	return &moment
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

// requireProvisionActor guards the provisioning surface: admin only
// (docs/API_ADDENDUM.md §1/§6.7 put student-account provisioning on
// groups.Admin; staff_tu is explicitly excluded — the router already gates the
// group; this is defense in depth, PRD §2.1).
func requireProvisionActor(actor Actor) error {
	if strings.TrimSpace(actor.ID) == "" {
		return ErrUnauthorized
	}
	if !hasRole(actor.Roles, RoleAdmin) {
		return ErrForbidden
	}
	return nil
}

// requireStudentActor guards the /my/* surface: a student-role login only, and
// never a client-supplied student id.
func requireStudentActor(actor Actor) error {
	if strings.TrimSpace(actor.ID) == "" {
		return ErrUnauthorized
	}
	if !hasRole(actor.Roles, RoleStudent) {
		return ErrForbidden
	}
	return nil
}

// cleanUsername validates an explicitly supplied username (the auth layer
// applies the same rules again for the column bounds).
func cleanUsername(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if utf8.RuneCountInString(value) > maxUsernameRunes || strings.IndexByte(value, 0) >= 0 || strings.ContainsAny(value, " \t\r\n") {
		return "", ErrInvalidInput
	}
	return value, nil
}

// cleanEmail validates the optional e-mail against the users column bounds.
func cleanEmail(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if utf8.RuneCountInString(value) > maxEmailRunes || strings.IndexByte(value, 0) >= 0 {
		return "", ErrInvalidInput
	}
	return value, nil
}

// parseDateRange converts the required from/to query pair into the pgdate
// parameters the shared attendance/discipline queries expect. Both bounds are
// mandatory and must stay inside maxRangeDays, exactly like
// GET /attendance/history (attendance.validateDateRange).
func parseDateRange(from, to string) (pgtype.Date, pgtype.Date, error) {
	start, err := parseDate(from)
	if err != nil {
		return pgtype.Date{}, pgtype.Date{}, ErrInvalidInput
	}
	end, err := parseDate(to)
	if err != nil || end.Time.Before(start.Time) {
		return pgtype.Date{}, pgtype.Date{}, ErrInvalidInput
	}
	if int(end.Time.Sub(start.Time).Hours()/24)+1 > maxRangeDays {
		return pgtype.Date{}, pgtype.Date{}, ErrInvalidInput
	}
	return start, end, nil
}

func parseDate(value string) (pgtype.Date, error) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	if err != nil {
		return pgtype.Date{}, err
	}
	return pgtype.Date{Time: t, Valid: true}, nil
}

func dateString(value pgtype.Date) string {
	if !value.Valid {
		return ""
	}
	return value.Time.Format("2006-01-02")
}

// normalizePage mirrors attendance.normalizePage: default 50, cap 100.
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

// attendancePercentage rounds to two decimals exactly like the attendance
// package so /my/attendance-summary matches the staff numbers.
func attendancePercentage(part, total int32) float64 {
	if total <= 0 {
		return 0
	}
	value := float64(part) / float64(total) * 100
	return float64(int(value*100+0.5)) / 100
}

// mapWriteError keeps the sentinel errors intact and translates the common
// database violations into them.
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

// accountView is the audit snapshot of a student login link. It never carries
// a credential token or a password (docs/API_ADDENDUM.md §0).
type accountView struct {
	StudentID     string `json:"student_id"`
	StudentNumber string `json:"student_number"`
	UserID        string `json:"user_id"`
	Username      string `json:"username"`
	Status        string `json:"status"`
}

func accountViewOf(studentID, studentNumber, userID, username, status string) accountView {
	return accountView{
		StudentID:     studentID,
		StudentNumber: studentNumber,
		UserID:        userID,
		Username:      username,
		Status:        status,
	}
}

// auditData writes an audit_logs row inside the caller's transaction
// (docs/API_ADDENDUM.md §0: account provisioning is a sensitive write).
func (s *Service) auditData(ctx context.Context, q *db.Queries, actorID pgtype.UUID, action string, entityID pgtype.UUID, before, after any, requestID string) error {
	var beforeData, afterData []byte
	var err error
	if before != nil {
		beforeData, err = json.Marshal(before)
		if err != nil {
			return fmt.Errorf("encode student account audit before data: %w", err)
		}
	}
	if after != nil {
		afterData, err = json.Marshal(after)
		if err != nil {
			return fmt.Errorf("encode student account audit after data: %w", err)
		}
	}
	_, err = q.CreateAuditLog(ctx, db.CreateAuditLogParams{
		ActorUserID: actorID,
		Action:      action,
		EntityType:  "student",
		EntityID:    entityID,
		RequestID:   optionalText(requestID),
		BeforeData:  string(beforeData),
		AfterData:   string(afterData),
	})
	if err != nil {
		return fmt.Errorf("create student account audit log: %w", err)
	}
	return nil
}
