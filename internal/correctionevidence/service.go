package correctionevidence

import (
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
)

// Config reuses the imports storage contract (docs/API_ADDENDUM.md §3):
// evidence files live in the same private directory, under the same size
// limit. No new config keys are introduced; the env fallbacks mirror the
// IFTER_IMPORTS_DIR / IFTER_IMPORTS_MAX_FILE_SIZE overrides of the config
// loader itself (same pattern as internal/dispensations).
type Config struct {
	Dir         string
	MaxFileSize int64
}

// ConfigFromEnvironment resolves the shared imports storage settings from the
// documented environment keys, falling back to the config defaults.
func ConfigFromEnvironment() Config {
	cfg := Config{Dir: "./storage/imports", MaxFileSize: 5 << 20}
	if dir := strings.TrimSpace(os.Getenv("IFTER_IMPORTS_DIR")); dir != "" {
		cfg.Dir = dir
	}
	if raw := strings.TrimSpace(os.Getenv("IFTER_IMPORTS_MAX_FILE_SIZE")); raw != "" {
		if size, err := strconv.ParseInt(raw, 10, 64); err == nil && size > 0 {
			cfg.MaxFileSize = size
		}
	}
	return cfg
}

// Service implements the correction-evidence use cases
// (handler -> usecase -> repository). Data scoping mirrors the attendance
// correction flow: admin is school-wide, teacher/duty_teacher only ever touch
// evidence of records in classes they own or are on duty for.
type Service struct {
	pool        *pgxpool.Pool
	queries     *db.Queries
	log         *slog.Logger
	dir         string
	maxFileSize int64
}

func NewService(pool *pgxpool.Pool, log *slog.Logger, cfg Config) *Service {
	if log == nil {
		log = slog.Default()
	}
	if strings.TrimSpace(cfg.Dir) == "" {
		cfg.Dir = "./storage/imports"
	}
	if cfg.MaxFileSize <= 0 {
		cfg.MaxFileSize = 5 << 20
	}
	dir, err := filepath.Abs(cfg.Dir)
	if err == nil {
		cfg.Dir = dir
	}
	return &Service{
		pool:        pool,
		queries:     db.New(pool),
		log:         log,
		dir:         cfg.Dir,
		maxFileSize: cfg.MaxFileSize,
	}
}

// MaxFileSize exposes the shared upload limit so the register file can size
// the multipart body limit (MaxFileSize + 1MiB).
func (s *Service) MaxFileSize() int64 {
	return s.maxFileSize
}
