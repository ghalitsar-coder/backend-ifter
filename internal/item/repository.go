package item

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
)

// Repository is the persistence contract the service depends on. Tests use a
// fake; production uses PostgresRepository.
type Repository interface {
	Create(ctx context.Context, name string) (Item, error)
	Get(ctx context.Context, id uuid.UUID) (Item, error)
	List(ctx context.Context, limit, offset int32) ([]Item, error)
	Count(ctx context.Context) (int64, error)
}

type PostgresRepository struct {
	queries *db.Queries
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{queries: db.New(pool)}
}

func (r *PostgresRepository) Create(ctx context.Context, name string) (Item, error) {
	row, err := r.queries.CreateItem(ctx, name)
	if err != nil {
		return Item{}, err
	}
	return toItem(row), nil
}

func (r *PostgresRepository) Get(ctx context.Context, id uuid.UUID) (Item, error) {
	row, err := r.queries.GetItem(ctx, id)
	if err != nil {
		return Item{}, err
	}
	return toItem(row), nil
}

func (r *PostgresRepository) List(ctx context.Context, limit, offset int32) ([]Item, error) {
	rows, err := r.queries.ListItems(ctx, db.ListItemsParams{Limit: limit, Offset: offset})
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(rows))
	for _, row := range rows {
		items = append(items, toItem(row))
	}
	return items, nil
}

func (r *PostgresRepository) Count(ctx context.Context) (int64, error) {
	return r.queries.CountItems(ctx)
}

func toItem(row db.Item) Item {
	return Item{
		ID:        row.ID,
		Name:      row.Name,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}
