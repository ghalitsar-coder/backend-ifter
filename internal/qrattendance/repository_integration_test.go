package qrattendance

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresQRAttendanceIntegration(t *testing.T) {
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

	const schoolID, creatorID, staffID = 1, 2, 2
	date := time.Date(2097, 11, 16, 0, 0, 0, 0, time.UTC)
	_, _ = pool.Exec(ctx, `DELETE FROM qr_sessions WHERE school_id = $1 AND tanggal = $2`, schoolID, date)
	_, _ = pool.Exec(ctx, `DELETE FROM staff_attendances WHERE staff_id = $1 AND tanggal = $2`, staffID, date)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM staff_attendances WHERE staff_id = $1 AND tanggal = $2`, staffID, date)
		_, _ = pool.Exec(context.Background(), `DELETE FROM qr_sessions WHERE school_id = $1 AND tanggal = $2`, schoolID, date)
	})

	repo := NewPostgresRepository(pool)
	lateAfter := "07:30:00"
	input := CreateSessionInput{Date: date.Format(dateLayout), Type: "masuk", OpenAt: "07:00", CloseAt: "09:00", LateAfter: &lateAfter}
	session, err := repo.CreateSession(ctx, schoolID, creatorID, "integration-test-secret", input, date, "07:00:00", "09:00:00", &lateAfter)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if session.ID <= 0 || session.TokenSecret != "integration-test-secret" || session.Status != "aktif" || session.Date != date.Format(dateLayout) {
		t.Fatalf("CreateSession() = %+v", session)
	}

	if _, err := repo.GetSession(ctx, session.ID, 999); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("GetSession(other school) error = %v, want ErrNoRows", err)
	}
	loaded, err := repo.GetSession(ctx, session.ID, schoolID)
	if err != nil || loaded.ID != session.ID || loaded.LateAfter == nil || *loaded.LateAfter != lateAfter {
		t.Fatalf("GetSession() = %+v, %v", loaded, err)
	}
	if staff, err := repo.FindActiveStaff(ctx, 2, schoolID); err != nil || staff != staffID {
		t.Fatalf("FindActiveStaff() = %d, %v; want %d", staff, err, staffID)
	}
	if _, err := repo.FindActiveStaff(ctx, 2, 999); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("FindActiveStaff(other school) error = %v, want ErrNoRows", err)
	}

	scannedAt := time.Date(2097, 11, 16, 7, 45, 0, 0, time.UTC)
	if err := repo.CheckIn(ctx, staffID, session.ID, date, scannedAt, "terlambat"); err != nil {
		t.Fatalf("CheckIn() error = %v", err)
	}
	if err := repo.CheckIn(ctx, staffID, session.ID, date, scannedAt, "hadir"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("duplicate CheckIn() error = %v, want ErrNoRows", err)
	}
	checkOutAt := scannedAt.Add(8 * time.Hour)
	if err := repo.CheckOut(ctx, staffID, session.ID, date, checkOutAt); err != nil {
		t.Fatalf("CheckOut() error = %v", err)
	}
	if err := repo.CheckOut(ctx, staffID, session.ID, date, checkOutAt); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("duplicate CheckOut() error = %v, want ErrNoRows", err)
	}
	if err := repo.CloseSession(ctx, session.ID, schoolID); err != nil {
		t.Fatalf("CloseSession() error = %v", err)
	}
	loaded, err = repo.GetSession(ctx, session.ID, schoolID)
	if err != nil || loaded.Status != "selesai" {
		t.Fatalf("closed session = %+v, %v; want selesai", loaded, err)
	}
}
