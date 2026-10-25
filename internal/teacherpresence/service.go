package teacherpresence

import (
	"context"
	"fmt"
	"log/slog"
	"time"
	_ "time/tzdata"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

// Service implements the teacher-presence use cases (handler -> usecase ->
// repository). The geofence verdict is always computed here, server-side.
type Service struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	log     *slog.Logger
}

func NewService(pool *pgxpool.Pool, log *slog.Logger) *Service {
	return &Service{
		pool:    pool,
		queries: db.New(pool),
		log:     log,
	}
}

// location resolves the school timezone from the shared settings row, the same
// source attendance uses for its local day boundary.
func (s *Service) location(ctx context.Context) (*time.Location, error) {
	settings, err := s.queries.GetSchoolSettings(ctx)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get school settings: %w", err)
	}
	loc, err := time.LoadLocation(settings.Timezone)
	if err != nil {
		return nil, fmt.Errorf("load school timezone: %w", err)
	}
	return loc, nil
}

// teacherIDForActor resolves the actor's own teacher profile. Presence is
// self-recorded only: an actor without a linked active teacher row cannot
// record or read presence (TECH_STACK.md §7, API_ADDENDUM.md §6.1).
func (s *Service) teacherIDForActor(ctx context.Context, actor Actor) (pgtype.UUID, error) {
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

func (s *Service) geofence(ctx context.Context) (db.SchoolGeofence, error) {
	row, err := s.queries.GetSchoolGeofence(ctx)
	if err != nil {
		if database.IsNotFound(err) {
			return db.SchoolGeofence{}, ErrNotFound
		}
		return db.SchoolGeofence{}, fmt.Errorf("get school geofence: %w", err)
	}
	return row, nil
}
