package employmenttype

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

// Service implements the employment-type use cases (handler -> usecase ->
// repository). The surface is master data (groups.MasterData: admin +
// staff_tu, enforced at the router); every write is audited with
// before/after snapshots (docs/API_ADDENDUM.md §0/§6.5).
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

// actorUUID resolves the authenticated principal writing the audit row.
func actorUUID(actor Actor) (pgtype.UUID, error) {
	id, err := parseUUID(actor.ID)
	if err != nil {
		return pgtype.UUID{}, ErrUnauthorized
	}
	return id, nil
}

// List returns every employment type (active and inactive) ordered by code.
func (s *Service) List(ctx context.Context) (TypeList, error) {
	rows, err := s.queries.ListEmploymentTypes(ctx)
	if err != nil {
		return TypeList{}, fmt.Errorf("list employment types: %w", err)
	}
	items := make([]EmploymentType, 0, len(rows))
	for _, row := range rows {
		items = append(items, typeFromModel(row))
	}
	return TypeList{Data: items}, nil
}

// Get returns one employment type for GET /employment-types/:id.
func (s *Service) Get(ctx context.Context, idValue string) (EmploymentType, error) {
	id, err := parseUUID(idValue)
	if err != nil {
		return EmploymentType{}, ErrInvalidInput
	}
	row, err := s.queries.GetEmploymentType(ctx, id)
	if err != nil {
		if database.IsNotFound(err) {
			return EmploymentType{}, ErrNotFound
		}
		return EmploymentType{}, fmt.Errorf("get employment type: %w", err)
	}
	return typeFromModel(row), nil
}

// Create stores a new employment type (POST /employment-types). Code must be
// unique (409 on collision); is_active defaults to true when the payload
// omits it. Audited as employment_type.create.
func (s *Service) Create(ctx context.Context, actor Actor, in TypeInput, requestID string) (EmploymentType, error) {
	actorID, err := actorUUID(actor)
	if err != nil {
		return EmploymentType{}, err
	}
	code, name, description, err := normalizeInput(in)
	if err != nil {
		return EmploymentType{}, err
	}
	var created EmploymentType
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		row, err := q.CreateEmploymentType(ctx, db.CreateEmploymentTypeParams{
			Code:        code,
			Name:        name,
			Description: optionalText(description),
		})
		if err != nil {
			return mapWriteError(fmt.Errorf("create employment type: %w", err))
		}
		if in.IsActive != nil && !*in.IsActive {
			row, err = q.SetEmploymentTypeStatus(ctx, db.SetEmploymentTypeStatusParams{
				IsActive: false,
				ID:       row.ID,
			})
			if err != nil {
				return mapWriteError(fmt.Errorf("deactivate new employment type: %w", err))
			}
		}
		created = typeFromModel(row)
		return s.auditData(ctx, q, actorID, "employment_type.create", row.ID, nil, created, requestID)
	})
	if err != nil {
		return EmploymentType{}, err
	}
	return created, nil
}

// Update replaces an employment type's payload (PUT /employment-types/:id).
// is_active is only changed when the payload carries it (COALESCE in SQL);
// status toggling otherwise owns PATCH :id/status. A code collision with
// another row is 409. Audited as employment_type.update with snapshots.
func (s *Service) Update(ctx context.Context, actor Actor, idValue string, in TypeInput, requestID string) (EmploymentType, error) {
	actorID, err := actorUUID(actor)
	if err != nil {
		return EmploymentType{}, err
	}
	id, err := parseUUID(idValue)
	if err != nil {
		return EmploymentType{}, ErrInvalidInput
	}
	code, name, description, err := normalizeInput(in)
	if err != nil {
		return EmploymentType{}, err
	}
	var updated EmploymentType
	nextActive := pgtype.Bool{}
	if in.IsActive != nil {
		nextActive = pgtype.Bool{Bool: *in.IsActive, Valid: true}
	}
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		before, err := q.GetEmploymentTypeForUpdate(ctx, id)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("lock employment type: %w", err)
		}
		row, err := q.UpdateEmploymentType(ctx, db.UpdateEmploymentTypeParams{
			Code:         code,
			Name:         name,
			Description:  optionalText(description),
			NextIsActive: nextActive,
			ID:           id,
		})
		if err != nil {
			return mapWriteError(fmt.Errorf("update employment type: %w", err))
		}
		updated = typeFromModel(row)
		return s.auditData(ctx, q, actorID, "employment_type.update", id,
			typeFromModel(before), updated, requestID)
	})
	if err != nil {
		return EmploymentType{}, err
	}
	return updated, nil
}

// SetStatus toggles is_active (PATCH /employment-types/:id/status). Audited
// as employment_type.status with before/after snapshots.
func (s *Service) SetStatus(ctx context.Context, actor Actor, idValue string, active bool, requestID string) (EmploymentType, error) {
	actorID, err := actorUUID(actor)
	if err != nil {
		return EmploymentType{}, err
	}
	id, err := parseUUID(idValue)
	if err != nil {
		return EmploymentType{}, ErrInvalidInput
	}
	var updated EmploymentType
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		before, err := q.GetEmploymentTypeForUpdate(ctx, id)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("lock employment type: %w", err)
		}
		row, err := q.SetEmploymentTypeStatus(ctx, db.SetEmploymentTypeStatusParams{
			IsActive: active,
			ID:       id,
		})
		if err != nil {
			return mapWriteError(fmt.Errorf("set employment type status: %w", err))
		}
		updated = typeFromModel(row)
		return s.auditData(ctx, q, actorID, "employment_type.status", id,
			typeFromModel(before), updated, requestID)
	})
	if err != nil {
		return EmploymentType{}, err
	}
	return updated, nil
}
