package dutyrotation

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

// Service implements the duty-rotation use cases (handler -> usecase ->
// repository). Server-side scoping rule (docs/API_ADDENDUM.md §6.3 + package
// spec): admin acts school-wide; teacher/duty_teacher may only view and write
// substitutions relevant to them — either the duty assignment is theirs or
// they are the named substitute. Relevance is resolved through the actor's
// own teachers row (teachers.user_id), never through a client claim.
type Service struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	log     *slog.Logger
}

func NewService(pool *pgxpool.Pool, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		pool:    pool,
		queries: db.New(pool),
		log:     log,
	}
}

func (s *Service) isStaffAdmin(actor Actor) bool {
	return hasRole(actor.Roles, "admin")
}

// actorTeacherID resolves the actor's own active teacher profile. A caller
// without one is forbidden from every non-admin rotation surface (mirrors
// internal/teacherpresence).
func (s *Service) actorTeacherID(ctx context.Context, actor Actor) (pgtype.UUID, error) {
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return pgtype.UUID{}, ErrUnauthorized
	}
	teacher, err := s.queries.GetTeacherByUserID(ctx, actorID)
	if err != nil {
		if database.IsNotFound(err) {
			return pgtype.UUID{}, ErrForbidden
		}
		return pgtype.UUID{}, fmt.Errorf("find teacher for actor: %w", err)
	}
	if teacher.Status != "active" {
		return pgtype.UUID{}, ErrForbidden
	}
	return teacher.ID, nil
}

// relevantTeacherID returns the teacher id a non-admin list must be scoped to.
// admin -> zero value (no filter, sees all).
func (s *Service) relevantTeacherID(ctx context.Context, actor Actor) (pgtype.UUID, error) {
	if s.isStaffAdmin(actor) {
		return pgtype.UUID{}, nil
	}
	return s.actorTeacherID(ctx, actor)
}

// checkRotationRelevance enforces "own assignments only" for non-admin actors:
// the actor must be the assignment's original teacher or the substitute.
func (s *Service) checkRotationRelevance(ctx context.Context, actor Actor, originalTeacherID, substituteTeacherID pgtype.UUID) error {
	if s.isStaffAdmin(actor) {
		return nil
	}
	teacherID, err := s.actorTeacherID(ctx, actor)
	if err != nil {
		return err
	}
	if teacherID.Valid && originalTeacherID.Valid && teacherID.Bytes == originalTeacherID.Bytes {
		return nil
	}
	if teacherID.Valid && substituteTeacherID.Valid && teacherID.Bytes == substituteTeacherID.Bytes {
		return nil
	}
	return ErrForbidden
}
