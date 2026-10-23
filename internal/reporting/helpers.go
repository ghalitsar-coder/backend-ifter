package reporting

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

type actorScope struct {
	allowAll  bool
	isAdmin   bool
	isTeacher bool
	isDuty    bool
	teacherID pgtype.UUID
	role      string
}

func resolveScope(ctx context.Context, q *db.Queries, actor Actor) (actorScope, error) {
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return actorScope{}, ErrUnauthorized
	}
	scope := actorScope{role: roleName(actor.Roles)}
	if scope.role == "" {
		return actorScope{}, ErrForbidden
	}
	if hasRole(actor.Roles, "admin") || hasRole(actor.Roles, "principal") {
		scope.allowAll = true
	}
	scope.isAdmin = hasRole(actor.Roles, "admin")
	if !hasRole(actor.Roles, "teacher") && !hasRole(actor.Roles, "duty_teacher") {
		return scope, nil
	}
	teacher, err := q.GetTeacherByUserID(ctx, actorID)
	if err != nil {
		if database.IsNotFound(err) {
			if scope.allowAll {
				return scope, nil
			}
			return actorScope{}, ErrForbidden
		}
		return actorScope{}, fmt.Errorf("find teacher for reporting scope: %w", err)
	}
	scope.teacherID = teacher.ID
	scope.isTeacher = hasRole(actor.Roles, "teacher")
	scope.isDuty = hasRole(actor.Roles, "duty_teacher")
	return scope, nil
}

func canReadClass(ctx context.Context, q *db.Queries, scope actorScope, classID pgtype.UUID, date pgtype.Date) error {
	if !classID.Valid {
		return nil
	}
	if scope.allowAll {
		return nil
	}
	if !scope.teacherID.Valid {
		return ErrForbidden
	}
	if scope.isTeacher {
		ok, err := q.HasHomeroomAssignmentForDate(ctx, db.HasHomeroomAssignmentForDateParams{
			ClassID:        classID,
			TeacherID:      scope.teacherID,
			AttendanceDate: date,
		})
		if err != nil {
			return fmt.Errorf("check homeroom scope: %w", err)
		}
		if ok {
			return nil
		}
	}
	if scope.isDuty {
		ok, err := q.HasDutyAssignmentForDate(ctx, db.HasDutyAssignmentForDateParams{
			TeacherID:      scope.teacherID,
			AttendanceDate: date,
		})
		if err != nil {
			return fmt.Errorf("check duty scope: %w", err)
		}
		if ok {
			return nil
		}
	}
	return ErrForbidden
}

func roleName(roles []string) string {
	switch {
	case hasRole(roles, "admin"):
		return "admin"
	case hasRole(roles, "principal"):
		return "principal"
	case hasRole(roles, "teacher"):
		return "teacher"
	case hasRole(roles, "duty_teacher"):
		return "duty_teacher"
	default:
		return ""
	}
}

func hasRole(roles []string, role string) bool {
	for _, candidate := range roles {
		if candidate == role {
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

func dateString(value pgtype.Date) string {
	if !value.Valid {
		return ""
	}
	return value.Time.Format("2006-01-02")
}

func timestampPtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time
	return &t
}

func timestamp(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}

// schoolLocation resolves the school timezone once per request so date filters
// can be translated into index-friendly timestamptz bounds.
func (s *Service) schoolLocation(ctx context.Context) (*time.Location, error) {
	settings, err := s.queries.GetSchoolSettings(ctx)
	if err != nil {
		if database.IsNotFound(err) {
			return time.UTC, nil
		}
		return nil, fmt.Errorf("get school settings: %w", err)
	}
	loc, err := time.LoadLocation(settings.Timezone)
	if err != nil {
		return time.UTC, nil
	}
	return loc, nil
}

func dayStart(date pgtype.Date, loc *time.Location) time.Time {
	return time.Date(date.Time.Year(), date.Time.Month(), date.Time.Day(), 0, 0, 0, 0, loc)
}

func timestampAt(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}

func optionalText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
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

func extractJSON(value any) json.RawMessage {
	var raw []byte
	switch typed := value.(type) {
	case string:
		raw = []byte(typed)
	case []byte:
		raw = typed
	}
	if len(raw) == 0 || !json.Valid(raw) {
		return nil
	}
	return json.RawMessage(raw)
}
