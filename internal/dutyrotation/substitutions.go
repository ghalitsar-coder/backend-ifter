package dutyrotation

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

// List returns duty_rotations for GET /duty-rotations?date=&weekday=. A
// duty_teacher/teacher only ever sees substitutions relevant to them (own
// assignment or own substitution); admin sees everything.
func (s *Service) List(ctx context.Context, actor Actor, filter RotationFilter) (RotationList, error) {
	var dateParam pgtype.Date
	if strings.TrimSpace(filter.Date) != "" {
		parsed, err := parseDate(filter.Date)
		if err != nil {
			return RotationList{}, ErrInvalidInput
		}
		dateParam = parsed
	}
	var weekdayParam pgtype.Int2
	if filter.Weekday != 0 {
		if filter.Weekday < 1 || filter.Weekday > 7 {
			return RotationList{}, ErrInvalidInput
		}
		weekdayParam = pgtype.Int2{Int16: int16(filter.Weekday), Valid: true}
	}
	relevant, err := s.relevantTeacherID(ctx, actor)
	if err != nil {
		return RotationList{}, err
	}
	rows, err := s.queries.ListDutyRotations(ctx, db.ListDutyRotationsParams{
		RotationDate:      dateParam,
		Weekday:           weekdayParam,
		RelevantTeacherID: relevant,
	})
	if err != nil {
		return RotationList{}, fmt.Errorf("list duty rotations: %w", err)
	}
	items := make([]RotationItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, RotationItem{
			ID:                    uuidString(row.ID),
			DutyAssignmentID:      uuidString(row.DutyAssignmentID),
			RotationDate:          dateValue(row.RotationDate),
			Weekday:               int(row.Weekday),
			OriginalTeacherID:     uuidString(row.OriginalTeacherID),
			OriginalTeacherName:   row.OriginalTeacherName,
			SubstituteTeacherID:   uuidString(row.SubstituteTeacherID),
			SubstituteTeacherName: row.SubstituteTeacherName,
			Status:                row.Status,
			Reason:                row.Reason,
			CreatedAt:             timestamp(row.CreatedAt),
		})
	}
	return RotationList{Data: items}, nil
}

// Create records a substitution (POST /duty-rotations) and audits it as
// duty.substitution. Validation, server-side (never client-trusted):
//   - the duty assignment exists (404 otherwise);
//   - rotation date falls inside the assignment window AND its ISO weekday
//     (1..7 Mon..Sun) matches the assignment weekday (400 otherwise);
//   - the substitute is an existing active teacher, not the original one;
//   - non-admin actors must be relevant to the assignment (own assignment or
//     be the substitute) — 403 otherwise;
//   - the unique triple (assignment, date, substitute) blocks duplicates (409).
func (s *Service) Create(ctx context.Context, actor Actor, in CreateRotationInput, requestID string) (RotationItem, error) {
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return RotationItem{}, ErrUnauthorized
	}
	assignmentID, err := parseUUID(in.DutyAssignmentID)
	if err != nil {
		return RotationItem{}, ErrInvalidInput
	}
	substituteID, err := parseUUID(in.SubstituteTeacherID)
	if err != nil {
		return RotationItem{}, ErrInvalidInput
	}
	rotationDate, err := parseDate(in.Date)
	if err != nil {
		return RotationItem{}, ErrInvalidInput
	}
	reason, err := cleanText(in.Reason, maxReasonRunes)
	if err != nil || reason == "" {
		return RotationItem{}, ErrInvalidInput
	}

	assignment, err := s.queries.GetDutyAssignmentForRotation(ctx, assignmentID)
	if err != nil {
		if database.IsNotFound(err) {
			return RotationItem{}, ErrNotFound
		}
		return RotationItem{}, fmt.Errorf("get duty assignment: %w", err)
	}
	if err := s.checkRotationRelevance(ctx, actor, assignment.TeacherID, substituteID); err != nil {
		return RotationItem{}, err
	}
	if err := validateRotationDate(rotationDate, assignment.StartDate, assignment.EndDate, assignment.Weekday); err != nil {
		return RotationItem{}, err
	}
	if assignment.TeacherID.Valid && substituteID.Valid && assignment.TeacherID.Bytes == substituteID.Bytes {
		return RotationItem{}, ErrInvalidInput
	}
	substitute, err := s.queries.GetTeacherForRotation(ctx, substituteID)
	if err != nil {
		if database.IsNotFound(err) {
			return RotationItem{}, ErrNotFound
		}
		return RotationItem{}, fmt.Errorf("get substitute teacher: %w", err)
	}
	if substitute.Status != "active" {
		return RotationItem{}, ErrInvalidInput
	}

	var createdID pgtype.UUID
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		created, err := q.CreateDutyRotation(ctx, db.CreateDutyRotationParams{
			DutyAssignmentID:    assignmentID,
			SubstituteTeacherID: substituteID,
			RotationDate:        rotationDate,
			Reason:              reason,
			CreatedBy:           actorID,
		})
		if err != nil {
			return mapWriteError(fmt.Errorf("create duty rotation: %w", err))
		}
		after, err := q.GetDutyRotation(ctx, created)
		if err != nil {
			return fmt.Errorf("read duty rotation after create: %w", err)
		}
		if err := s.auditData(ctx, q, actorID, "duty.substitution", "duty_rotation", created,
			nil, newRotationView(after), requestID); err != nil {
			return fmt.Errorf("write duty substitution audit: %w", err)
		}
		createdID = created
		return nil
	})
	if err != nil {
		return RotationItem{}, err
	}
	return s.fetchRotation(ctx, createdID)
}

// Cancel flips a substitution to status=cancelled (PATCH /duty-rotations/:id/
// cancel). The original assignment is untouched; cancelling twice is a 409.
// An optional reason replaces the stored one; blank keeps it. Audited as
// duty.substitution.cancel.
func (s *Service) Cancel(ctx context.Context, actor Actor, id string, in CancelInput, requestID string) (RotationItem, error) {
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return RotationItem{}, ErrUnauthorized
	}
	rotationID, err := parseUUID(id)
	if err != nil {
		return RotationItem{}, ErrInvalidInput
	}
	reason, err := cleanText(in.Reason, maxReasonRunes)
	if err != nil {
		return RotationItem{}, ErrInvalidInput
	}

	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		before, err := q.GetDutyRotation(ctx, rotationID)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("get duty rotation before cancel: %w", err)
		}
		if err := s.checkRotationRelevance(ctx, actor, before.OriginalTeacherID, before.SubstituteTeacherID); err != nil {
			return err
		}
		if before.Status != StatusSubstituted {
			return ErrConflict
		}
		if _, err := q.CancelDutyRotation(ctx, db.CancelDutyRotationParams{
			Reason: reason,
			ID:     rotationID,
		}); err != nil {
			if database.IsNotFound(err) {
				return ErrConflict
			}
			return mapWriteError(fmt.Errorf("cancel duty rotation: %w", err))
		}
		after, err := q.GetDutyRotation(ctx, rotationID)
		if err != nil {
			return fmt.Errorf("read duty rotation after cancel: %w", err)
		}
		if err := s.auditData(ctx, q, actorID, "duty.substitution.cancel", "duty_rotation", rotationID,
			newRotationView(before), newRotationView(after), requestID); err != nil {
			return fmt.Errorf("write duty substitution cancel audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return RotationItem{}, err
	}
	return s.fetchRotation(ctx, rotationID)
}

// validateRotationDate checks the rotation date sits in the assignment window
// and carries the assignment's weekday (1..7 Mon..Sun).
func validateRotationDate(date pgtype.Date, start pgtype.Date, end pgtype.Date, weekday int16) error {
	day := date.Time
	if !start.Valid || day.Before(start.Time) {
		return ErrInvalidInput
	}
	if end.Valid && day.After(end.Time) {
		return ErrInvalidInput
	}
	if isoWeekday(day) != int(weekday) {
		return ErrInvalidInput
	}
	return nil
}

func (s *Service) fetchRotation(ctx context.Context, id pgtype.UUID) (RotationItem, error) {
	row, err := s.queries.GetDutyRotation(ctx, id)
	if err != nil {
		if database.IsNotFound(err) {
			return RotationItem{}, ErrNotFound
		}
		return RotationItem{}, fmt.Errorf("fetch duty rotation: %w", err)
	}
	return RotationItem{
		ID:                    uuidString(row.ID),
		DutyAssignmentID:      uuidString(row.DutyAssignmentID),
		RotationDate:          dateValue(row.RotationDate),
		Weekday:               int(row.Weekday),
		OriginalTeacherID:     uuidString(row.OriginalTeacherID),
		OriginalTeacherName:   row.OriginalTeacherName,
		SubstituteTeacherID:   uuidString(row.SubstituteTeacherID),
		SubstituteTeacherName: row.SubstituteTeacherName,
		Status:                row.Status,
		Reason:                row.Reason,
		CreatedAt:             timestamp(row.CreatedAt),
	}, nil
}
