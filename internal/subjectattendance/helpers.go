package subjectattendance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
	"github.com/google/uuid"
)

func hasRole(roles []string, role string) bool {
	for _, candidate := range roles {
		if candidate == role {
			return true
		}
	}
	return false
}

func hasAnyRole(roles []string, candidates ...string) bool {
	for _, candidate := range candidates {
		if hasRole(roles, candidate) {
			return true
		}
	}
	return false
}

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
	id := uuidString(value)
	return &id
}

func optionalText(value string) pgtype.Text {
	trimmed := strings.TrimSpace(value)
	return pgtype.Text{String: trimmed, Valid: trimmed != ""}
}

func textPtr(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	text := value.String
	return &text
}

func timestamp(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time.UTC()
}

func timestampPtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	at := value.Time.UTC()
	return &at
}

func dateString(value pgtype.Date) string {
	if !value.Valid {
		return ""
	}
	return value.Time.Format("2006-01-02")
}

func parseDate(value string) (pgtype.Date, error) {
	parsed, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	if err != nil {
		return pgtype.Date{}, err
	}
	return pgtype.Date{Time: parsed, Valid: true}, nil
}

// parseDateRange keeps the same mandatory from/to contract as
// GET /attendance/history (both bounds required, <= maxRangeDays).
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

func attendancePercentage(part, total int64) float64 {
	if total <= 0 {
		return 0
	}
	value := float64(part) / float64(total) * 100
	return float64(int(value*100+0.5)) / 100
}

func cleanText(value string, max int) (string, error) {
	trimmed := strings.TrimSpace(value)
	if utf8.RuneCountInString(trimmed) > max {
		return "", ErrInvalidInput
	}
	return trimmed, nil
}

func requireMasterActor(actor Actor) error {
	if strings.TrimSpace(actor.ID) == "" {
		return ErrUnauthorized
	}
	if !hasAnyRole(actor.Roles, RoleAdmin, RoleStaffTU) {
		return ErrForbidden
	}
	return nil
}

func requireReadActor(actor Actor) error {
	if strings.TrimSpace(actor.ID) == "" {
		return ErrUnauthorized
	}
	if !hasAnyRole(actor.Roles, RoleAdmin, RoleStaffTU, RoleTeacher, RoleDutyTeacher, RolePrincipal) {
		return ErrForbidden
	}
	return nil
}

// actorScope is the session/assignment read-write scope: unrestriced staff see
// the whole school, a plain teacher is pinned to their own teacher profile.
type actorScope struct {
	teacherID pgtype.UUID
	restrict  bool
}

func (s actorScope) allows(teacherID pgtype.UUID) bool {
	return !s.restrict || s.teacherID == teacherID
}

// resolveScope decides school-wide vs own-assignment access. writes excludes
// principal and staff_tu; reads includes them (all of them school-wide).
func (s *Service) resolveScope(ctx context.Context, actor Actor, write bool) (actorScope, error) {
	if strings.TrimSpace(actor.ID) == "" {
		return actorScope{}, ErrUnauthorized
	}
	if write {
		if !hasAnyRole(actor.Roles, RoleAdmin, RoleTeacher, RoleDutyTeacher) {
			return actorScope{}, ErrForbidden
		}
	} else if !hasAnyRole(actor.Roles, RoleAdmin, RoleStaffTU, RoleTeacher, RoleDutyTeacher, RolePrincipal) {
		return actorScope{}, ErrForbidden
	}
	if hasAnyRole(actor.Roles, RoleAdmin, RoleDutyTeacher) || (!write && hasAnyRole(actor.Roles, RoleStaffTU, RolePrincipal)) {
		return actorScope{}, nil
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return actorScope{}, ErrUnauthorized
	}
	teacher, err := s.queries.GetTeacherByUserID(ctx, actorID)
	if err != nil {
		if database.IsNotFound(err) {
			return actorScope{}, ErrForbidden
		}
		return actorScope{}, fmt.Errorf("find teacher profile: %w", err)
	}
	return actorScope{teacherID: teacher.ID, restrict: true}, nil
}

func mapWriteError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrUnauthorized), errors.Is(err, ErrForbidden),
		errors.Is(err, ErrInvalidInput), errors.Is(err, ErrNotFound),
		errors.Is(err, ErrConflict):
		return err
	case database.IsUniqueViolation(err):
		return ErrConflict
	case isClosedSessionViolation(err):
		return ErrConflict
	case database.IsForeignKeyViolation(err),
		database.IsExclusionViolation(err),
		database.IsCheckViolation(err),
		database.IsNotNullViolation(err):
		return ErrInvalidInput
	default:
		return err
	}
}

// isClosedSessionViolation recognizes the migration 000020 trigger guard, so a
// closed session answers 409 even if a writer bypassed the service pre-check.
func isClosedSessionViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "P0001" &&
		strings.Contains(pgErr.Message, "attendance session is closed")
}

func (s *Service) audit(ctx context.Context, q *db.Queries, actorID pgtype.UUID, action, entityType string, entityID pgtype.UUID, before, after any, requestID string) error {
	var beforeData, afterData []byte
	var err error
	if before != nil {
		beforeData, err = json.Marshal(before)
		if err != nil {
			return fmt.Errorf("encode %s audit before data: %w", action, err)
		}
	}
	if after != nil {
		afterData, err = json.Marshal(after)
		if err != nil {
			return fmt.Errorf("encode %s audit after data: %w", action, err)
		}
	}
	if _, err := q.CreateAuditLog(ctx, db.CreateAuditLogParams{
		ActorUserID: actorID,
		Action:      action,
		EntityType:  entityType,
		EntityID:    entityID,
		RequestID:   optionalText(requestID),
		BeforeData:  string(beforeData),
		AfterData:   string(afterData),
	}); err != nil {
		return fmt.Errorf("create %s audit log: %w", action, err)
	}
	return nil
}
