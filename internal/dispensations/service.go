package dispensations

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

// Config reuses the imports storage contract (docs/API_ADDENDUM.md §3):
// documents live in the same private directory, under the same size limit.
// No new config keys are introduced; the env fallbacks mirror the
// IFTER_IMPORTS_DIR / IFTER_IMPORTS_MAX_FILE_SIZE overrides of the config
// loader itself.
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

// Service implements the dispensation use cases (handler -> usecase ->
// repository), including server-side scoping: teachers only ever see and
// verify dispensations of classes they are currently homeroom teacher of.
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

// resolveScope computes what the actor may read: admin/staff_tu/duty_teacher/
// principal are school-wide; a plain teacher is scoped to their currently
// homeroom-taught classes (empty = sees nothing).
type readScope struct {
	all      bool
	classIDs []pgtype.UUID
}

func (s *Service) resolveReadScope(ctx context.Context, actor Actor) (readScope, error) {
	if hasRole(actor.Roles, "admin") || hasRole(actor.Roles, "staff_tu") ||
		hasRole(actor.Roles, "duty_teacher") || hasRole(actor.Roles, "principal") {
		return readScope{all: true}, nil
	}
	if !hasRole(actor.Roles, "teacher") {
		return readScope{}, ErrForbidden
	}
	teacherID, _, err := s.teacherProfile(ctx, actor)
	if err != nil {
		return readScope{}, err
	}
	classIDs, err := s.queries.ListCurrentHomeroomClassIDs(ctx, teacherID)
	if err != nil {
		return readScope{}, fmt.Errorf("list homeroom classes: %w", err)
	}
	return readScope{classIDs: classIDs}, nil
}

func (s *Service) teacherProfile(ctx context.Context, actor Actor) (pgtype.UUID, bool, error) {
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return pgtype.UUID{}, false, ErrUnauthorized
	}
	teacher, err := s.queries.GetTeacherByUserID(ctx, actorID)
	if err != nil {
		if database.IsNotFound(err) {
			return pgtype.UUID{}, false, nil
		}
		return pgtype.UUID{}, false, fmt.Errorf("find teacher for actor: %w", err)
	}
	return teacher.ID, teacher.Status == "active", nil
}

// studentIDForActor resolves the actor's own student through the
// students.user_id link added by migration 000015 (student-accounts package).
// The column is queried with raw SQL because it is not part of this package's
// sqlc schema; before 000015 is applied the lookup fails and /my/* answers
// 403 with ErrAccountNotLinked.
func (s *Service) studentIDForActor(ctx context.Context, actor Actor) (pgtype.UUID, error) {
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return pgtype.UUID{}, ErrUnauthorized
	}
	var studentID pgtype.UUID
	err = s.pool.QueryRow(ctx,
		"SELECT id FROM students WHERE user_id = $1 AND status = 'active'", actorID,
	).Scan(&studentID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "42703" {
			return pgtype.UUID{}, ErrAccountNotLinked
		}
		if database.IsNotFound(err) {
			return pgtype.UUID{}, ErrAccountNotLinked
		}
		return pgtype.UUID{}, fmt.Errorf("find student for actor: %w", err)
	}
	if !studentID.Valid {
		return pgtype.UUID{}, ErrAccountNotLinked
	}
	return studentID, nil
}

// checkClassAccess enforces the teacher own-class rule on single-resource
// reads and document downloads; school-wide roles always pass.
func checkClassAccess(scope readScope, classID pgtype.UUID) error {
	if scope.all {
		return nil
	}
	for _, candidate := range scope.classIDs {
		if candidate.Valid && classID.Valid && candidate.Bytes == classID.Bytes {
			return nil
		}
	}
	return ErrForbidden
}
