package maintenance

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

// Config controls the daily retention job. A zero retention disables the
// corresponding cleanup.
type Config struct {
	AuditLogRetention     time.Duration
	RefreshTokenRetention time.Duration
	Interval              time.Duration
}

type Service struct {
	pool             *pgxpool.Pool
	log              *slog.Logger
	auditRetention   time.Duration
	refreshRetention time.Duration
	interval         time.Duration
}

func NewService(pool *pgxpool.Pool, log *slog.Logger, cfg Config) *Service {
	return &Service{
		pool:             pool,
		log:              log,
		auditRetention:   cfg.AuditLogRetention,
		refreshRetention: cfg.RefreshTokenRetention,
		interval:         cfg.Interval,
	}
}

// Run cleans up once on startup and then every interval until ctx is
// cancelled.
func (s *Service) Run(ctx context.Context) {
	if err := s.Cleanup(ctx); err != nil {
		s.log.Warn("maintenance_cleanup_failed", "error", err)
	}
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.Cleanup(ctx); err != nil {
				s.log.Warn("maintenance_cleanup_failed", "error", err)
			}
		}
	}
}

func (s *Service) Cleanup(ctx context.Context) error {
	if s.auditRetention <= 0 && s.refreshRetention <= 0 {
		return nil
	}
	now := time.Now()
	return database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		if s.auditRetention > 0 {
			// The append-only trigger only allows DELETE inside a transaction
			// that opts in through this local setting.
			if _, err := tx.Exec(ctx, "SET LOCAL ifter.audit_retention = 'on'"); err != nil {
				return fmt.Errorf("enable audit retention: %w", err)
			}
			deleted, err := q.DeleteAuditLogsBefore(ctx, pgtype.Timestamptz{
				Time:  now.Add(-s.auditRetention),
				Valid: true,
			})
			if err != nil {
				return fmt.Errorf("delete expired audit logs: %w", err)
			}
			if deleted > 0 {
				s.log.Info("audit_logs_retention_deleted", "rows", deleted)
			}
		}
		if s.refreshRetention > 0 {
			deleted, err := q.DeleteExpiredRefreshTokensBefore(ctx, pgtype.Timestamptz{
				Time:  now.Add(-s.refreshRetention),
				Valid: true,
			})
			if err != nil {
				return fmt.Errorf("delete expired refresh tokens: %w", err)
			}
			if deleted > 0 {
				s.log.Info("refresh_tokens_retention_deleted", "rows", deleted)
			}
		}
		return nil
	})
}
