package devices

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

func requireCardManager(actor Actor) error {
	if !hasRole(actor.Roles, "admin") && !hasRole(actor.Roles, "staff_tu") {
		return ErrForbidden
	}
	return nil
}

// GetStudentCard returns the card mapped to a student (GET /students/:id/card);
// an unmapped student answers 200 with card_uid null.
func (s *Service) GetStudentCard(ctx context.Context, actor Actor, studentIDValue string) (StudentCard, error) {
	if err := requireCardManager(actor); err != nil {
		return StudentCard{}, err
	}
	studentID, err := parseUUID(studentIDValue)
	if err != nil {
		return StudentCard{}, ErrInvalidInput
	}
	if _, err := s.queries.GetStudent(ctx, studentID); err != nil {
		if database.IsNotFound(err) {
			return StudentCard{}, ErrStudentNotFound
		}
		return StudentCard{}, fmt.Errorf("get student: %w", err)
	}
	card, err := s.queries.GetStudentCard(ctx, studentID)
	if err != nil {
		if database.IsNotFound(err) {
			return StudentCard{StudentID: uuidString(studentID)}, nil
		}
		return StudentCard{}, fmt.Errorf("get student card: %w", err)
	}
	return studentCardOf(card), nil
}

// SetStudentCard maps a card UID to a student (PUT /students/:id/card). PUT
// replaces the mapping: a null (or omitted) card_uid clears it. Card UIDs are
// unique across students, so a card already mapped elsewhere answers 409.
// Audited as student.card.
func (s *Service) SetStudentCard(ctx context.Context, actor Actor, studentIDValue string, cardUID *string, requestID string) (StudentCard, error) {
	if err := requireCardManager(actor); err != nil {
		return StudentCard{}, err
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return StudentCard{}, ErrUnauthorized
	}
	studentID, err := parseUUID(studentIDValue)
	if err != nil {
		return StudentCard{}, ErrInvalidInput
	}
	normalized := ""
	clear := cardUID == nil
	if !clear {
		normalized, err = normalizeCardUID(*cardUID)
		if err != nil {
			return StudentCard{}, err
		}
		clear = normalized == ""
	}
	result := StudentCard{StudentID: uuidString(studentID)}
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		if _, err := q.GetStudent(ctx, studentID); err != nil {
			if database.IsNotFound(err) {
				return ErrStudentNotFound
			}
			return fmt.Errorf("get student: %w", err)
		}
		before, err := q.GetStudentCard(ctx, studentID)
		hasBefore := err == nil
		if err != nil && !database.IsNotFound(err) {
			return fmt.Errorf("get student card: %w", err)
		}
		if clear {
			result.CardUID = nil
			if !hasBefore {
				return nil
			}
			if _, err := q.DeleteStudentCard(ctx, studentID); err != nil {
				return mapWriteError(fmt.Errorf("delete student card: %w", err))
			}
			return s.auditData(ctx, q, actorID, "student", "student.card", studentID, studentCardOf(before), result, requestID)
		}
		result.CardUID = &normalized
		if hasBefore && before.CardUid == normalized {
			return nil
		}
		card, err := q.UpsertStudentCard(ctx, db.UpsertStudentCardParams{
			StudentID: studentID,
			CardUid:   normalized,
		})
		if err != nil {
			return mapWriteError(fmt.Errorf("upsert student card: %w", err))
		}
		result = studentCardOf(card)
		if !hasBefore {
			return s.auditData(ctx, q, actorID, "student", "student.card", studentID, nil, result, requestID)
		}
		return s.auditData(ctx, q, actorID, "student", "student.card", studentID, studentCardOf(before), result, requestID)
	})
	if err != nil {
		return StudentCard{}, err
	}
	return result, nil
}

func studentCardOf(row db.StudentCard) StudentCard {
	cardUID := row.CardUid
	return StudentCard{StudentID: uuidString(row.StudentID), CardUID: &cardUID}
}
