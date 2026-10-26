package maintenance_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ghalitsar-coder/backend-ifter/internal/maintenance"
)

func TestCleanupPrunesExpiredRows(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `INSERT INTO users (username, full_name, status)
		VALUES ('retention-user', 'Retention User', 'system')`); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	var userID string
	if err := pool.QueryRow(ctx, `SELECT id FROM users WHERE username = 'retention-user'`).Scan(&userID); err != nil {
		t.Fatalf("read user id: %v", err)
	}

	insertAuditLog(t, pool, "2020-01-01T00:00:00Z")
	insertAuditLog(t, pool, time.Now().Format(time.RFC3339))
	insertRefreshToken(t, pool, userID, stringsHash("a"), "2020-01-01T00:00:00Z")
	insertRefreshToken(t, pool, userID, stringsHash("b"), time.Now().Add(7*24*time.Hour).Format(time.RFC3339))

	service := maintenance.NewService(pool, discardLogger(), maintenance.Config{
		AuditLogRetention:     30 * 24 * time.Hour,
		RefreshTokenRetention: 24 * time.Hour,
		Interval:              time.Hour,
	})
	if err := service.Cleanup(ctx); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}

	if got := count(t, pool, "audit_logs"); got != 1 {
		t.Fatalf("audit_logs rows = %d, want 1", got)
	}
	if got := count(t, pool, "refresh_tokens"); got != 1 {
		t.Fatalf("refresh_tokens rows = %d, want 1", got)
	}

	if _, err := pool.Exec(ctx, "DELETE FROM audit_logs"); err == nil {
		t.Fatal("direct DELETE on audit_logs succeeded, want append-only error")
	}
}

func TestCleanupDisabledKeepsRows(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	insertAuditLog(t, pool, "2020-01-01T00:00:00Z")

	service := maintenance.NewService(pool, discardLogger(), maintenance.Config{Interval: time.Hour})
	if err := service.Cleanup(ctx); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if got := count(t, pool, "audit_logs"); got != 1 {
		t.Fatalf("audit_logs rows = %d, want 1", got)
	}
}

func insertAuditLog(t *testing.T, pool *pgxpool.Pool, createdAt string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `INSERT INTO audit_logs (action, entity_type, created_at)
		VALUES ('test.retention', 'test', $1::timestamptz)`, createdAt); err != nil {
		t.Fatalf("insert audit log: %v", err)
	}
}

func insertRefreshToken(t *testing.T, pool *pgxpool.Pool, userID, hash, expiresAt string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3::timestamptz)`, userID, hash, expiresAt); err != nil {
		t.Fatalf("insert refresh token: %v", err)
	}
}

func stringsHash(seed string) string {
	hash := ""
	for len(hash) < 64 {
		hash += seed
	}
	return hash[:64]
}

func count(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var rows int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&rows); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return rows
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	adminConfig, err := pgxpool.ParseConfig(adminURL)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	admin, err := pgxpool.NewWithConfig(ctx, adminConfig)
	if err != nil {
		t.Fatalf("connect admin database: %v", err)
	}
	name := fmt.Sprintf("ifter_maintenance_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		admin.Close()
		t.Fatalf("create test database: %v", err)
	}
	var pool *pgxpool.Pool
	t.Cleanup(func() {
		if pool != nil {
			pool.Close()
		}
		_, _ = admin.Exec(context.Background(), "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
		admin.Close()
	})
	testConfig, err := pgxpool.ParseConfig(adminURL)
	if err != nil {
		t.Fatalf("parse test database config: %v", err)
	}
	testConfig.ConnConfig.Database = name
	testConfig.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err = pgxpool.NewWithConfig(ctx, testConfig)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	paths, err := filepath.Glob(filepath.Join("..", "..", "db", "migrations", "*.up.sql"))
	if err != nil {
		t.Fatalf("find migrations: %v", err)
	}
	sort.Strings(paths)
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %s: %v", path, err)
		}
		if _, err := pool.Exec(ctx, string(body)); err != nil {
			t.Fatalf("apply migration %s: %v", path, err)
		}
	}
	return pool
}
