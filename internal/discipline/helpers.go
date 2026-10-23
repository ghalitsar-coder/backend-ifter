package discipline

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

const maxReportDays = 366

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

func optionalText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}

func textString(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
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

func intPtr(value pgtype.Int4) *int {
	if !value.Valid {
		return nil
	}
	converted := int(value.Int32)
	return &converted
}

func textPtr(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
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

func validStatus(value string) bool {
	switch value {
	case "new", "under_review", "in_followup", "completed", "cancelled":
		return true
	default:
		return false
	}
}

func validCategoryAuto(value string) bool {
	return value == "" || value == "late"
}

func validateDateRange(from, to string) (pgtype.Date, pgtype.Date, error) {
	start, err := parseDate(from)
	if err != nil {
		return pgtype.Date{}, pgtype.Date{}, ErrInvalidInput
	}
	end, err := parseDate(to)
	if err != nil {
		return pgtype.Date{}, pgtype.Date{}, ErrInvalidInput
	}
	if end.Time.Before(start.Time) {
		return pgtype.Date{}, pgtype.Date{}, ErrInvalidInput
	}
	if end.Time.Sub(start.Time) > maxReportDays*24*time.Hour {
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

type actorScope struct {
	allowAll  bool
	isTeacher bool
	isDuty    bool
	teacherID pgtype.UUID
}

func resolveScope(ctx context.Context, q *db.Queries, actor Actor) (actorScope, error) {
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return actorScope{}, ErrUnauthorized
	}
	scope := actorScope{}
	if hasRole(actor.Roles, "admin") || hasRole(actor.Roles, "principal") {
		scope.allowAll = true
	}
	if !hasRole(actor.Roles, "teacher") && !hasRole(actor.Roles, "duty_teacher") {
		return scope, nil
	}
	teacher, err := q.GetTeacherByUserID(ctx, actorID)
	if err != nil {
		if database.IsNotFound(err) {
			return scope, nil
		}
		return actorScope{}, fmt.Errorf("find teacher for discipline scope: %w", err)
	}
	scope.teacherID = teacher.ID
	scope.isTeacher = hasRole(actor.Roles, "teacher")
	scope.isDuty = hasRole(actor.Roles, "duty_teacher")
	return scope, nil
}

func hasRole(roles []string, role string) bool {
	for _, candidate := range roles {
		if candidate == role {
			return true
		}
	}
	return false
}

func canReadClass(ctx context.Context, q *db.Queries, actor Actor, classID pgtype.UUID, date pgtype.Date) error {
	scope, err := resolveScope(ctx, q, actor)
	if err != nil {
		return err
	}
	if scope.allowAll {
		return nil
	}
	if !scope.teacherID.Valid {
		return ErrForbidden
	}
	if scope.isTeacher {
		homeroom, err := q.HasHomeroomAssignmentForDate(ctx, db.HasHomeroomAssignmentForDateParams{
			ClassID:        classID,
			TeacherID:      scope.teacherID,
			AttendanceDate: date,
		})
		if err != nil {
			return fmt.Errorf("check homeroom discipline scope: %w", err)
		}
		if homeroom {
			return nil
		}
	}
	if scope.isDuty {
		duty, err := q.HasDutyAssignmentForDate(ctx, db.HasDutyAssignmentForDateParams{
			TeacherID:      scope.teacherID,
			AttendanceDate: date,
		})
		if err != nil {
			return fmt.Errorf("check duty discipline scope: %w", err)
		}
		if duty {
			return nil
		}
	}
	return ErrForbidden
}

func canWriteClass(ctx context.Context, q *db.Queries, actor Actor, classID pgtype.UUID, date pgtype.Date) error {
	if hasRole(actor.Roles, "admin") {
		return nil
	}
	if !hasRole(actor.Roles, "teacher") && !hasRole(actor.Roles, "duty_teacher") {
		return ErrForbidden
	}
	return canReadClass(ctx, q, actor, classID, date)
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

func categoryFromModel(row db.DisciplineCategory) Category {
	return Category{
		ID:                     uuidString(row.ID),
		Code:                   textString(row.Code),
		Name:                   row.Name,
		Description:            textString(row.Description),
		DefaultPoints:          intPtr(row.DefaultPoints),
		AutoOnAttendanceStatus: textPtr(row.AutoOnAttendanceStatus),
		IsActive:               row.IsActive,
		CreatedAt:              timestamp(row.CreatedAt),
		UpdatedAt:              timestamp(row.UpdatedAt),
	}
}

func categoryAudit(value Category) map[string]any {
	return map[string]any{
		"code":                      value.Code,
		"name":                      value.Name,
		"description":               value.Description,
		"default_points":            value.DefaultPoints,
		"auto_on_attendance_status": value.AutoOnAttendanceStatus,
		"is_active":                 value.IsActive,
	}
}

func recordFromGetRow(row db.GetDisciplineRecordRow) Record {
	return Record{
		ID:                 uuidString(row.ID),
		StudentID:          uuidString(row.StudentID),
		StudentNumber:      row.StudentNumber,
		StudentName:        row.StudentName,
		ClassID:            uuidString(row.ClassID),
		ClassName:          row.ClassName,
		CategoryID:         uuidString(row.CategoryID),
		CategoryName:       row.CategoryName,
		OccurredAt:         timestamp(row.OccurredAt),
		OccurredDate:       dateString(row.OccurredDate),
		Description:        row.Description,
		Points:             row.Points,
		Status:             row.Status,
		AttendanceRecordID: stringPtrOrNil(row.AttendanceRecordID),
		RecordedByUserID:   uuidString(row.RecordedByUserID),
		RecordedByName:     row.RecordedByName,
		CreatedAt:          timestamp(row.CreatedAt),
		UpdatedAt:          timestamp(row.UpdatedAt),
	}
}

func recordFromListRow(row db.ListDisciplineRecordsRow) Record {
	return Record{
		ID:                 uuidString(row.ID),
		StudentID:          uuidString(row.StudentID),
		StudentNumber:      row.StudentNumber,
		StudentName:        row.StudentName,
		ClassID:            uuidString(row.ClassID),
		ClassName:          row.ClassName,
		CategoryID:         uuidString(row.CategoryID),
		CategoryName:       row.CategoryName,
		OccurredAt:         timestamp(row.OccurredAt),
		OccurredDate:       dateString(row.OccurredDate),
		Description:        row.Description,
		Points:             row.Points,
		Status:             row.Status,
		AttendanceRecordID: stringPtrOrNil(row.AttendanceRecordID),
		RecordedByUserID:   uuidString(row.RecordedByUserID),
		RecordedByName:     row.RecordedByName,
		CreatedAt:          timestamp(row.CreatedAt),
		UpdatedAt:          timestamp(row.UpdatedAt),
	}
}

func followupFromRow(row db.ListDisciplineFollowupsRow) Followup {
	return Followup{
		ID:            uuidString(row.ID),
		RecordID:      uuidString(row.DisciplineRecordID),
		Status:        row.Status,
		Notes:         textString(row.Notes),
		UpdatedByID:   uuidString(row.UpdatedByUserID),
		UpdatedByName: row.UpdatedByName,
		CreatedAt:     timestamp(row.CreatedAt),
	}
}

func stringPtrOrNil(value pgtype.UUID) *string {
	if !value.Valid {
		return nil
	}
	converted := uuidString(value)
	return &converted
}

func (s *Service) auditData(ctx context.Context, q *db.Queries, actorID pgtype.UUID, action, entityType string, entityID pgtype.UUID, before, after any, reason, requestID string) error {
	var beforeData, afterData []byte
	var err error
	if before != nil {
		beforeData, err = json.Marshal(before)
		if err != nil {
			return err
		}
	}
	if after != nil {
		afterData, err = json.Marshal(after)
		if err != nil {
			return err
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
		Reason:      optionalText(reason),
	})
	if err != nil {
		return fmt.Errorf("create audit log: %w", err)
	}
	return nil
}
