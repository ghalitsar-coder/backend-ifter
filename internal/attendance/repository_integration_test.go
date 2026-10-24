package attendance

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Runs only when TEST_DATABASE_URL points at a migrated database with the demo
// seed loaded (db/seed/demo.sql). Locally:
//
//	TEST_DATABASE_URL='postgres://app:app@localhost:5432/app?sslmode=disable' go test ./internal/attendance/
func TestPostgresRepositoryIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	t.Cleanup(pool.Close)

	var seeded bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM classes WHERE id = 1)").Scan(&seeded); err != nil {
		t.Fatalf("check demo seed: %v", err)
	}
	if !seeded {
		t.Skip("demo seed is not loaded; run db/seed/demo.sql")
	}

	repo := NewPostgresRepository(pool)

	classes, err := repo.ListClasses(ctx, 1)
	if err != nil || len(classes) == 0 || classes[0].NamaKelas != "1A" {
		t.Fatalf("ListClasses() = %+v, %v; want the seeded class 1A first", classes, err)
	}

	if outside, err := repo.ListClasses(ctx, 999); err != nil || len(outside) != 0 {
		t.Fatalf("ListClasses(school 999) = %+v, %v; want empty, nil", outside, err)
	}
	if in, err := repo.ClassInSchool(ctx, 1, 999); err != nil || in {
		t.Fatalf("ClassInSchool(school 999) = %v, %v; want false, nil", in, err)
	}

	tanggal := time.Date(2099, 12, 31, 0, 0, 0, 0, time.UTC)
	_, _ = pool.Exec(ctx, "DELETE FROM attendance_sessions WHERE class_id = 1 AND tanggal = $1", tanggal)

	staffID, err := repo.StaffIDByUserID(ctx, 1)
	if err != nil {
		t.Fatalf("StaffIDByUserID() error = %v", err)
	}
	if allowed, err := repo.CanRecord(ctx, staffID, 1, 1, nil); err != nil || !allowed {
		t.Fatalf("CanRecord() = %v, %v; want true, nil", allowed, err)
	}
	if allowed, err := repo.CanRecord(ctx, staffID, 1, 999, nil); err != nil || allowed {
		t.Fatalf("CanRecord(school 999) = %v, %v; want false, nil", allowed, err)
	}

	session, err := repo.CreateSession(ctx, 1, staffID, nil, tanggal)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM attendance_sessions WHERE id = $1", session.ID)
	})

	again, err := repo.CreateSession(ctx, 1, staffID, nil, tanggal)
	if err != nil || again.ID != session.ID {
		t.Fatalf("idempotent CreateSession() = %d, %v; want id %d", again.ID, err, session.ID)
	}

	entries := []AttendanceInput{
		{StudentID: 1, Status: "hadir"},
		{StudentID: 2, Status: "sakit"},
		{StudentID: 3, Status: "izin"},
	}
	enrolled, err := repo.CountEnrolledStudents(ctx, 1, []int64{1, 2, 3})
	if err != nil || enrolled != 3 {
		t.Fatalf("CountEnrolledStudents() = %d, %v; want 3, nil", enrolled, err)
	}
	if err := repo.RecordAttendances(ctx, session.ID, entries); err != nil {
		t.Fatalf("RecordAttendances() error = %v", err)
	}

	rows, err := repo.ListAttendances(ctx, session.ID, 1)
	if err != nil {
		t.Fatalf("ListAttendances() error = %v", err)
	}
	if len(rows) != 3 || rows[0].Status == nil || *rows[0].Status != "hadir" {
		t.Fatalf("ListAttendances() = %+v, want 3 rows starting with hadir", rows)
	}

	locked, err := repo.LockSession(ctx, session.ID)
	if err != nil || !locked.IsLocked {
		t.Fatalf("LockSession() = %+v, %v; want locked", locked, err)
	}
	if err := repo.RecordAttendances(ctx, session.ID, entries); !errors.Is(err, ErrLocked) {
		t.Fatalf("RecordAttendances() after lock error = %v, want ErrLocked", err)
	}
}
