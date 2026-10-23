package masterdata

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
)

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
