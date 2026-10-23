package item

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

// TestPostgresRepository runs only when TEST_DATABASE_URL points at a database
// where the migrations have been applied. CI provisions one; locally use
// `make db-up && make migrate-up`.
func TestPostgresRepository(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	// t.Cleanup runs LIFO, so the delete below runs before the pool closes.
	t.Cleanup(pool.Close)

	repo := NewPostgresRepository(pool)
	name := "integration-" + uuid.NewString()

	created, err := repo.Create(ctx, name)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM items WHERE id = $1", created.ID)
	})

	if created.ID == uuid.Nil || created.Name != name {
		t.Fatalf("Create() = %+v, want a persisted item named %q", created, name)
	}

	found, err := repo.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if found.ID != created.ID {
		t.Errorf("Get() id = %s, want %s", found.ID, created.ID)
	}

	if _, err := repo.Get(ctx, uuid.New()); !database.IsNotFound(err) {
		t.Errorf("Get(unknown) error = %v, want not found", err)
	}

	items, err := repo.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) == 0 {
		t.Error("List() returned no items")
	}

	total, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if total < 1 {
		t.Errorf("Count() = %d, want at least 1", total)
	}
}
