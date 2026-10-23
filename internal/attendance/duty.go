package attendance

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

func (s *Service) ListDutyAssignments(ctx context.Context, actor Actor, filter DutyAssignmentFilter) ([]DutyAssignment, error) {
	if !hasRole(actor.Roles, "admin") {
		return nil, ErrForbidden
	}
	teacherID, err := optionalUUID(filter.TeacherID)
	if err != nil {
		return nil, ErrInvalidInput
	}
	if filter.Weekday < 0 || filter.Weekday > 7 {
		return nil, ErrInvalidInput
	}
	params := db.ListDutyAssignmentsParams{TeacherID: teacherID}
	if filter.Weekday > 0 {
		params.Weekday = pgtype.Int2{Int16: filter.Weekday, Valid: true}
	}
	if filter.Date != "" {
		date, err := parseDate(filter.Date)
		if err != nil {
			return nil, ErrInvalidInput
		}
		params.ActiveDate = date
	}
	rows, err := s.queries.ListDutyAssignments(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list duty assignments: %w", err)
	}
	items := make([]DutyAssignment, 0, len(rows))
	for _, row := range rows {
		items = append(items, dutyAssignmentFromListRow(row))
	}
	return items, nil
}

func (s *Service) CreateDutyAssignment(ctx context.Context, actor Actor, input DutyAssignmentInput, requestID string) (DutyAssignment, error) {
	if !hasRole(actor.Roles, "admin") {
		return DutyAssignment{}, ErrForbidden
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return DutyAssignment{}, ErrUnauthorized
	}
	teacherID, err := parseUUID(input.TeacherID)
	if err != nil {
		return DutyAssignment{}, ErrInvalidInput
	}
	if input.Weekday < 1 || input.Weekday > 7 {
		return DutyAssignment{}, ErrInvalidInput
	}
	startDate, err := parseDate(input.StartDate)
	if err != nil {
		return DutyAssignment{}, ErrInvalidInput
	}
	endDate, err := optionalDate(input.EndDate)
	if err != nil || (endDate.Valid && endDate.Time.Before(startDate.Time)) {
		return DutyAssignment{}, ErrInvalidInput
	}
	var created db.DutyAssignment
	var teacherName string
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		teacher, err := q.GetTeacher(ctx, teacherID)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("get duty teacher: %w", err)
		}
		teacherName = teacher.FullName
		created, err = q.CreateDutyAssignment(ctx, db.CreateDutyAssignmentParams{
			TeacherID: teacherID,
			Weekday:   input.Weekday,
			StartDate: startDate,
			EndDate:   endDate,
			CreatedBy: actorID,
		})
		if err != nil {
			return mapWriteError(fmt.Errorf("create duty assignment: %w", err))
		}
		if err := s.auditData(ctx, q, actorID, "attendance.duty_create", "duty_assignment", created.ID, nil, map[string]any{
			"teacher_id": input.TeacherID,
			"weekday":    input.Weekday,
			"start_date": dateString(startDate),
			"end_date":   dateString(endDate),
		}, requestID); err != nil {
			return fmt.Errorf("write duty assignment audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return DutyAssignment{}, err
	}
	return DutyAssignment{
		ID:          uuidString(created.ID),
		TeacherID:   uuidString(created.TeacherID),
		TeacherName: teacherName,
		Weekday:     created.Weekday,
		StartDate:   dateString(created.StartDate),
		EndDate:     datePointer(created.EndDate),
		CreatedAt:   timestamp(created.CreatedAt),
	}, nil
}

func (s *Service) EndDutyAssignment(ctx context.Context, actor Actor, idValue, endDateValue, requestID string) (DutyAssignment, error) {
	if !hasRole(actor.Roles, "admin") {
		return DutyAssignment{}, ErrForbidden
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return DutyAssignment{}, ErrUnauthorized
	}
	id, err := parseUUID(idValue)
	if err != nil {
		return DutyAssignment{}, ErrInvalidInput
	}
	endDate, err := parseDate(endDateValue)
	if err != nil {
		return DutyAssignment{}, ErrInvalidInput
	}
	var updated db.DutyAssignment
	var teacherName string
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		assignment, err := q.GetDutyAssignmentForUpdate(ctx, id)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("get duty assignment: %w", err)
		}
		if endDate.Time.Before(assignment.StartDate.Time) {
			return ErrInvalidInput
		}
		teacher, err := q.GetTeacher(ctx, assignment.TeacherID)
		if err != nil {
			return fmt.Errorf("get duty teacher: %w", err)
		}
		teacherName = teacher.FullName
		updated, err = q.UpdateDutyAssignmentEnd(ctx, db.UpdateDutyAssignmentEndParams{
			EndDate: endDate,
			ID:      id,
		})
		if err != nil {
			return mapWriteError(fmt.Errorf("end duty assignment: %w", err))
		}
		if err := s.auditData(ctx, q, actorID, "attendance.duty_update", "duty_assignment", id, map[string]any{
			"end_date": dateString(assignment.EndDate),
		}, map[string]any{
			"end_date": dateString(endDate),
		}, requestID); err != nil {
			return fmt.Errorf("write duty assignment audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return DutyAssignment{}, err
	}
	return DutyAssignment{
		ID:          uuidString(updated.ID),
		TeacherID:   uuidString(updated.TeacherID),
		TeacherName: teacherName,
		Weekday:     updated.Weekday,
		StartDate:   dateString(updated.StartDate),
		EndDate:     datePointer(updated.EndDate),
		CreatedAt:   timestamp(updated.CreatedAt),
	}, nil
}

func dutyAssignmentFromListRow(row db.ListDutyAssignmentsRow) DutyAssignment {
	return DutyAssignment{
		ID:          uuidString(row.ID),
		TeacherID:   uuidString(row.TeacherID),
		TeacherName: row.TeacherName,
		Weekday:     row.Weekday,
		StartDate:   dateString(row.StartDate),
		EndDate:     datePointer(row.EndDate),
		CreatedAt:   timestamp(row.CreatedAt),
	}
}

func optionalDate(value string) (pgtype.Date, error) {
	if value == "" {
		return pgtype.Date{}, nil
	}
	return parseDate(value)
}

func datePointer(value pgtype.Date) *string {
	if !value.Valid {
		return nil
	}
	formatted := dateString(value)
	return &formatted
}
