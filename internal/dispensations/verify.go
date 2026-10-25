package dispensations

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

// Verify is the decision write (POST /dispensations/:id/verify). Admin and
// duty_teacher act school-wide; a teacher may only verify dispensations of a
// class they are currently homeroom teacher of — checked server-side against
// homeroom_assignments, never against a client claim. The transition
// pending -> approved|rejected is audited as dispensation.verify.
func (s *Service) Verify(ctx context.Context, actor Actor, id string, in VerifyInput, requestID string) (Item, error) {
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return Item{}, ErrUnauthorized
	}
	dispensationID, err := parseUUID(id)
	if err != nil {
		return Item{}, ErrInvalidInput
	}
	if !validDecision(in.Status) {
		return Item{}, ErrInvalidInput
	}
	note, err := cleanText(in.Note, 500)
	if err != nil {
		return Item{}, ErrInvalidInput
	}

	var verified db.Dispensation
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		before, err := q.GetDispensation(ctx, dispensationID)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("get dispensation before verify: %w", err)
		}
		if err := s.checkVerifyScope(ctx, q, actor, before.ClassID); err != nil {
			return err
		}
		if before.Status != StatusPending {
			return ErrConflict
		}
		updated, err := q.VerifyDispensation(ctx, db.VerifyDispensationParams{
			Status:     in.Status,
			VerifiedBy: actorID,
			Note:       note,
			ID:         dispensationID,
		})
		if err != nil {
			if database.IsNotFound(err) {
				return ErrConflict
			}
			return mapWriteError(fmt.Errorf("verify dispensation: %w", err))
		}
		if err := s.auditData(ctx, q, actorID, "dispensation.verify", "dispensation", dispensationID,
			verificationView(before.Status, before.VerifiedBy, before.VerifiedAt, before.Note),
			verificationView(updated.Status, updated.VerifiedBy, updated.VerifiedAt, updated.Note),
			requestID); err != nil {
			return fmt.Errorf("write dispensation verify audit: %w", err)
		}
		verified = updated
		return nil
	})
	if err != nil {
		return Item{}, err
	}
	return s.fetchItem(ctx, verified.ID)
}

// checkVerifyScope allows admin and duty_teacher school-wide; a teacher must
// be the active homeroom teacher of the dispensation's class.
func (s *Service) checkVerifyScope(ctx context.Context, q *db.Queries, actor Actor, classID pgtype.UUID) error {
	if hasRole(actor.Roles, "admin") || hasRole(actor.Roles, "duty_teacher") {
		return nil
	}
	if !hasRole(actor.Roles, "teacher") {
		return ErrForbidden
	}
	if !classID.Valid {
		return ErrForbidden
	}
	teacherID, active, err := s.teacherProfile(ctx, actor)
	if err != nil {
		return err
	}
	if !active {
		return ErrForbidden
	}
	homeroom, err := q.GetCurrentHomeroomForClass(ctx, classID)
	if err != nil {
		if database.IsNotFound(err) {
			return ErrForbidden
		}
		return fmt.Errorf("check homeroom scope: %w", err)
	}
	if homeroom.TeacherID != teacherID {
		return ErrForbidden
	}
	return nil
}
