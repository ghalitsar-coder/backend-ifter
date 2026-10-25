package subjectattendance

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
)

// Service implements per-subject attendance (F8, migration 000020): subjects,
// teaching assignments, lesson sessions, per-student session records and the
// recap. Daily attendance (internal/attendance) stays untouched.
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
