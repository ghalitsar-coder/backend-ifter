package dutyrotation

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

// CreateUniform records a uniform note (POST /uniform-reports). Validation is
// server-side: scope must be student|teacher; a teacher-scope note never
// carries a student target (mirrors the migration 000011 CHECK); the teacher
// must exist (404) and be active (400); the target student, when given, must
// exist (404).
func (s *Service) CreateUniform(ctx context.Context, actor Actor, in CreateUniformInput) (UniformItem, error) {
	if _, err := parseUUID(actor.ID); err != nil {
		return UniformItem{}, ErrUnauthorized
	}
	forDate, err := parseDate(in.ForDate)
	if err != nil {
		return UniformItem{}, ErrInvalidInput
	}
	teacherID, err := parseUUID(in.TeacherID)
	if err != nil {
		return UniformItem{}, ErrInvalidInput
	}
	scope := strings.TrimSpace(in.Scope)
	if !validScope(scope) {
		return UniformItem{}, ErrInvalidInput
	}
	targetStudentID, hasTarget, err := optionalUUID(in.TargetStudentID)
	if err != nil {
		return UniformItem{}, ErrInvalidInput
	}
	if hasTarget && scope == ScopeTeacher {
		return UniformItem{}, ErrInvalidInput
	}
	note, err := cleanText(in.Note, maxNoteRunes)
	if err != nil {
		return UniformItem{}, ErrInvalidInput
	}

	teacher, err := s.queries.GetTeacherForRotation(ctx, teacherID)
	if err != nil {
		if database.IsNotFound(err) {
			return UniformItem{}, ErrNotFound
		}
		return UniformItem{}, fmt.Errorf("get uniform report teacher: %w", err)
	}
	if teacher.Status != "active" {
		return UniformItem{}, ErrInvalidInput
	}
	if hasTarget {
		student, err := s.queries.GetStudentForUniform(ctx, targetStudentID)
		if err != nil {
			if database.IsNotFound(err) {
				return UniformItem{}, ErrNotFound
			}
			return UniformItem{}, fmt.Errorf("get uniform report target student: %w", err)
		}
		if student.Status != "active" {
			return UniformItem{}, ErrInvalidInput
		}
	}

	row, err := s.queries.CreateUniformReport(ctx, db.CreateUniformReportParams{
		ForDate:         forDate,
		TeacherID:       teacherID,
		Scope:           scope,
		TargetStudentID: targetStudentID,
		Note:            note,
	})
	if err != nil {
		return UniformItem{}, mapWriteError(fmt.Errorf("create uniform report: %w", err))
	}
	return uniformItemFromRow(row), nil
}

// ListUniform returns uniform notes for GET /uniform-reports?date&teacher_id.
func (s *Service) ListUniform(ctx context.Context, actor Actor, filter UniformFilter) (UniformList, error) {
	if _, err := parseUUID(actor.ID); err != nil {
		return UniformList{}, ErrUnauthorized
	}
	var dateParam pgtype.Date
	if strings.TrimSpace(filter.Date) != "" {
		parsed, err := parseDate(filter.Date)
		if err != nil {
			return UniformList{}, ErrInvalidInput
		}
		dateParam = parsed
	}
	teacherParam, _, err := optionalUUID(filter.TeacherID)
	if err != nil {
		return UniformList{}, ErrInvalidInput
	}
	rows, err := s.queries.ListUniformReports(ctx, db.ListUniformReportsParams{
		ForDate:   dateParam,
		TeacherID: teacherParam,
	})
	if err != nil {
		return UniformList{}, fmt.Errorf("list uniform reports: %w", err)
	}
	items := make([]UniformItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, uniformItemFromRow(row))
	}
	return UniformList{Data: items}, nil
}

func uniformItemFromRow(row db.UniformReport) UniformItem {
	return UniformItem{
		ID:              uuidString(row.ID),
		ForDate:         dateValue(row.ForDate),
		TeacherID:       uuidString(row.TeacherID),
		Scope:           row.Scope,
		TargetStudentID: uuidPtr(row.TargetStudentID),
		Note:            textPtr(row.Note),
		CreatedAt:       timestamp(row.CreatedAt),
	}
}

func uuidPtr(value pgtype.UUID) *string {
	if !value.Valid {
		return nil
	}
	text := uuidString(value)
	return &text
}
