package discipline

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
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

func (s *Service) schoolLocation(ctx context.Context) (*time.Location, error) {
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

func (s *Service) parseOccurredAt(ctx context.Context, value string) (pgtype.Timestamptz, pgtype.Date, error) {
	loc, err := s.schoolLocation(ctx)
	if err != nil {
		return pgtype.Timestamptz{}, pgtype.Date{}, err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return pgtype.Timestamptz{}, pgtype.Date{}, ErrInvalidInput
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		local := parsed.In(loc)
		date := pgtype.Date{Time: time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC), Valid: true}
		return pgtype.Timestamptz{Time: parsed.UTC(), Valid: true}, date, nil
	}
	for _, layout := range []string{
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
	} {
		if parsed, err := time.ParseInLocation(layout, value, loc); err == nil {
			date := pgtype.Date{Time: time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, time.UTC), Valid: true}
			return pgtype.Timestamptz{Time: parsed.UTC(), Valid: true}, date, nil
		}
	}
	if parsed, err := time.ParseInLocation("2006-01-02", value, loc); err == nil {
		parsed = time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 12, 0, 0, 0, loc)
		date := pgtype.Date{Time: time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, time.UTC), Valid: true}
		return pgtype.Timestamptz{Time: parsed.UTC(), Valid: true}, date, nil
	}
	return pgtype.Timestamptz{}, pgtype.Date{}, ErrInvalidInput
}

func optionalDateRange(from, to string) (pgtype.Date, pgtype.Date, error) {
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	if from == "" && to == "" {
		return pgtype.Date{}, pgtype.Date{}, nil
	}
	if from == "" || to == "" {
		return pgtype.Date{}, pgtype.Date{}, ErrInvalidInput
	}
	return validateDateRange(from, to)
}

func (s *Service) ListCategories(ctx context.Context, actor Actor, includeInactive bool) ([]Category, error) {
	if includeInactive && !hasRole(actor.Roles, "admin") && !hasRole(actor.Roles, "principal") {
		return nil, ErrForbidden
	}
	isActive := pgtype.Bool{Bool: true, Valid: true}
	if includeInactive {
		isActive = pgtype.Bool{}
	}
	rows, err := s.queries.ListDisciplineCategories(ctx, isActive)
	if err != nil {
		return nil, fmt.Errorf("list discipline categories: %w", err)
	}
	items := make([]Category, 0, len(rows))
	for _, row := range rows {
		items = append(items, categoryFromModel(row))
	}
	return items, nil
}

func (s *Service) CreateCategory(ctx context.Context, actor Actor, input CategoryInput, requestID string) (Category, error) {
	if !hasRole(actor.Roles, "admin") {
		return Category{}, ErrForbidden
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return Category{}, ErrUnauthorized
	}
	name, code, description, points, autoStatus, err := normalizeCategoryInput(input)
	if err != nil {
		return Category{}, err
	}
	var created db.DisciplineCategory
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		created, err = q.CreateDisciplineCategory(ctx, db.CreateDisciplineCategoryParams{
			Code:                   optionalText(code),
			Name:                   name,
			Description:            optionalText(description),
			DefaultPoints:          toInt4(points),
			AutoOnAttendanceStatus: optionalText(autoStatus),
		})
		if err != nil {
			return mapWriteError(fmt.Errorf("create discipline category: %w", err))
		}
		if err := s.auditData(ctx, q, actorID, "discipline.category_create", "discipline_category", created.ID, nil, categoryAudit(categoryFromModel(created)), "", requestID); err != nil {
			return fmt.Errorf("write discipline category audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return Category{}, err
	}
	return categoryFromModel(created), nil
}

func (s *Service) UpdateCategory(ctx context.Context, actor Actor, idValue string, input CategoryInput, requestID string) (Category, error) {
	if !hasRole(actor.Roles, "admin") {
		return Category{}, ErrForbidden
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return Category{}, ErrUnauthorized
	}
	id, err := parseUUID(idValue)
	if err != nil {
		return Category{}, ErrInvalidInput
	}
	name, code, description, points, autoStatus, err := normalizeCategoryInput(input)
	if err != nil {
		return Category{}, err
	}
	var updated db.DisciplineCategory
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		before, err := q.GetDisciplineCategoryForUpdate(ctx, id)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("get discipline category: %w", err)
		}
		updated, err = q.UpdateDisciplineCategory(ctx, db.UpdateDisciplineCategoryParams{
			Code:                   optionalText(code),
			Name:                   name,
			Description:            optionalText(description),
			DefaultPoints:          toInt4(points),
			AutoOnAttendanceStatus: optionalText(autoStatus),
			ID:                     id,
		})
		if err != nil {
			return mapWriteError(fmt.Errorf("update discipline category: %w", err))
		}
		if err := s.auditData(ctx, q, actorID, "discipline.category_update", "discipline_category", id, categoryAudit(categoryFromModel(before)), categoryAudit(categoryFromModel(updated)), "", requestID); err != nil {
			return fmt.Errorf("write discipline category audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return Category{}, err
	}
	return categoryFromModel(updated), nil
}

func (s *Service) SetCategoryStatus(ctx context.Context, actor Actor, idValue string, active bool, requestID string) (Category, error) {
	if !hasRole(actor.Roles, "admin") {
		return Category{}, ErrForbidden
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return Category{}, ErrUnauthorized
	}
	id, err := parseUUID(idValue)
	if err != nil {
		return Category{}, ErrInvalidInput
	}
	var updated db.DisciplineCategory
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		before, err := q.GetDisciplineCategoryForUpdate(ctx, id)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("get discipline category: %w", err)
		}
		updated, err = q.SetDisciplineCategoryStatus(ctx, db.SetDisciplineCategoryStatusParams{
			IsActive: active,
			ID:       id,
		})
		if err != nil {
			return mapWriteError(fmt.Errorf("set discipline category status: %w", err))
		}
		if err := s.auditData(ctx, q, actorID, "discipline.category_status", "discipline_category", id, categoryAudit(categoryFromModel(before)), categoryAudit(categoryFromModel(updated)), "", requestID); err != nil {
			return fmt.Errorf("write discipline category audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return Category{}, err
	}
	return categoryFromModel(updated), nil
}

func normalizeCategoryInput(input CategoryInput) (string, string, string, *int, string, error) {
	name, err := requiredText(input.Name, 100)
	if err != nil {
		return "", "", "", nil, "", err
	}
	code, err := cleanText(input.Code, 50)
	if err != nil {
		return "", "", "", nil, "", err
	}
	description, err := cleanText(input.Description, 2000)
	if err != nil {
		return "", "", "", nil, "", err
	}
	points := input.DefaultPoints
	if points != nil && *points < 0 {
		return "", "", "", nil, "", ErrInvalidInput
	}
	autoStatus := strings.TrimSpace(input.AutoOnAttendanceStatus)
	if !validCategoryAuto(autoStatus) {
		return "", "", "", nil, "", ErrInvalidInput
	}
	return name, code, description, points, autoStatus, nil
}

func toInt4(value *int) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(*value), Valid: true}
}
