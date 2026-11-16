package qrattendance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type fakeRepository struct {
	createFn   func(context.Context, int64, int64, string, CreateSessionInput, time.Time, string, string, *string) (SessionRecord, error)
	getFn      func(context.Context, int64, int64) (SessionRecord, error)
	closeFn    func(context.Context, int64, int64) error
	staffFn    func(context.Context, int64, int64) (int64, error)
	checkInFn  func(context.Context, int64, int64, time.Time, time.Time, string) error
	checkOutFn func(context.Context, int64, int64, time.Time, time.Time) error
}

func (f fakeRepository) CreateSession(ctx context.Context, schoolID, creatorID int64, secret string, input CreateSessionInput, date time.Time, openAt, closeAt string, lateAfter *string) (SessionRecord, error) {
	return f.createFn(ctx, schoolID, creatorID, secret, input, date, openAt, closeAt, lateAfter)
}

func (f fakeRepository) GetSession(ctx context.Context, id, schoolID int64) (SessionRecord, error) {
	return f.getFn(ctx, id, schoolID)
}

func (f fakeRepository) CloseSession(ctx context.Context, id, schoolID int64) error {
	return f.closeFn(ctx, id, schoolID)
}

func (f fakeRepository) FindActiveStaff(ctx context.Context, userID, schoolID int64) (int64, error) {
	return f.staffFn(ctx, userID, schoolID)
}

func (f fakeRepository) CheckIn(ctx context.Context, staffID, sessionID int64, date, scannedAt time.Time, status string) error {
	return f.checkInFn(ctx, staffID, sessionID, date, scannedAt, status)
}

func (f fakeRepository) CheckOut(ctx context.Context, staffID, sessionID int64, date, scannedAt time.Time) error {
	return f.checkOutFn(ctx, staffID, sessionID, date, scannedAt)
}

func TestCreateSessionValidatesBeforeRepository(t *testing.T) {
	svc := NewService(fakeRepository{})
	input := CreateSessionInput{Date: "2026-11-16", Type: "masuk", OpenAt: "07:00", CloseAt: "08:00"}
	if _, err := svc.CreateSession(context.Background(), SessionCreator{}, input); !errors.Is(err, ErrForbidden) {
		t.Fatalf("error = %v, want ErrForbidden", err)
	}
	creator := SessionCreator{UserID: 1, SchoolID: 1}
	input.Type = "invalid"
	if _, err := svc.CreateSession(context.Background(), creator, input); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("error = %v, want ErrInvalidSession", err)
	}
	input.Type = "masuk"
	input.CloseAt = "06:00"
	if _, err := svc.CreateSession(context.Background(), creator, input); !errors.Is(err, ErrInvalidTime) {
		t.Fatalf("error = %v, want ErrInvalidTime", err)
	}
}

func TestCurrentCodeAndScanValidateRotatingSignature(t *testing.T) {
	now := time.Date(2026, 11, 16, 7, 30, 15, 0, time.FixedZone("WIB", 7*60*60))
	lateAfter := "07:30:00"
	session := SessionRecord{
		Session:  Session{ID: 9, Date: now.Format(dateLayout), Type: "masuk", OpenAt: "07:00:00", CloseAt: "08:00:00", LateAfter: &lateAfter, RotationSeconds: 30, Status: "aktif"},
		SchoolID: 2, TokenSecret: "secret-for-test",
	}
	var recordedStatus string
	var lookupSchool int64
	repo := fakeRepository{
		getFn:   func(context.Context, int64, int64) (SessionRecord, error) { return session, nil },
		staffFn: func(_ context.Context, _, schoolID int64) (int64, error) { lookupSchool = schoolID; return 42, nil },
		checkInFn: func(_ context.Context, staffID, sessionID int64, _, _ time.Time, status string) error {
			if staffID != 42 || sessionID != session.ID {
				t.Fatalf("check-in target = staff %d/session %d", staffID, sessionID)
			}
			recordedStatus = status
			return nil
		},
	}
	svc := NewService(repo)
	svc.now = func() time.Time { return now }

	code, err := svc.CurrentCode(context.Background(), session.ID, session.SchoolID)
	if err != nil {
		t.Fatalf("CurrentCode() error = %v", err)
	}
	if code.Payload == "" || len(code.ImagePNG) < len("data:image/png;base64,") || code.ValidUntil.IsZero() {
		t.Fatalf("QR code result incomplete: %+v", code)
	}

	result, err := svc.Scan(context.Background(), ScanInput{UserID: 7, SchoolID: session.SchoolID, Payload: code.Payload})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if result.StaffID != 42 || result.Status != "terlambat" || lookupSchool != session.SchoolID || recordedStatus != "terlambat" {
		t.Fatalf("scan result = %+v, school lookup=%d status=%q", result, lookupSchool, recordedStatus)
	}

	tampered := code.Payload[:len(code.Payload)-1] + "A"
	if _, err := svc.Scan(context.Background(), ScanInput{UserID: 7, SchoolID: session.SchoolID, Payload: tampered}); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("tampered Scan() error = %v, want ErrInvalidCode", err)
	}

	stalePayload := signPayload(session.ID, now.Unix()/30-1, session.TokenSecret)
	if _, err := svc.Scan(context.Background(), ScanInput{UserID: 7, SchoolID: session.SchoolID, Payload: stalePayload}); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("stale Scan() error = %v, want ErrInvalidCode", err)
	}
}

func TestScanRejectsStaffOutsideSchool(t *testing.T) {
	now := time.Date(2026, 11, 16, 7, 30, 15, 0, time.FixedZone("WIB", 7*60*60))
	session := SessionRecord{Session: Session{ID: 9, Date: now.Format(dateLayout), Type: "masuk", OpenAt: "07:00:00", CloseAt: "08:00:00", Status: "aktif", RotationSeconds: 0}, SchoolID: 2, TokenSecret: "secret"}
	svc := NewService(fakeRepository{
		getFn:   func(context.Context, int64, int64) (SessionRecord, error) { return session, nil },
		staffFn: func(context.Context, int64, int64) (int64, error) { return 0, pgx.ErrNoRows },
	})
	svc.now = func() time.Time { return now }
	payload := signPayload(session.ID, 0, session.TokenSecret)
	if _, err := svc.Scan(context.Background(), ScanInput{UserID: 99, SchoolID: 2, Payload: payload}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("Scan() error = %v, want ErrForbidden", err)
	}
}
