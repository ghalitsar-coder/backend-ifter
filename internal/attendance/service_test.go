package attendance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type fakeRepository struct {
	staffFn   func(ctx context.Context, userID int64) (int64, error)
	canFn     func(ctx context.Context, staffID, classID int64, subjectID *int64) (bool, error)
	createFn  func(ctx context.Context, classID, recordedBy int64, subjectID *int64, tanggal time.Time) (Session, error)
	getFn     func(ctx context.Context, id int64) (Session, error)
	listFn    func(ctx context.Context, sessionID, classID int64) ([]Attendance, error)
	countFn   func(ctx context.Context, classID int64, studentIDs []int64) (int64, error)
	recordFn  func(ctx context.Context, sessionID int64, attendances []AttendanceInput) error
	lockFn    func(ctx context.Context, id int64) (Session, error)
	canCalled bool
}

func (f *fakeRepository) StaffIDByUserID(ctx context.Context, userID int64) (int64, error) {
	return f.staffFn(ctx, userID)
}

func (f *fakeRepository) CanRecord(ctx context.Context, staffID, classID int64, subjectID *int64) (bool, error) {
	f.canCalled = true
	return f.canFn(ctx, staffID, classID, subjectID)
}

func (f *fakeRepository) CreateSession(ctx context.Context, classID, recordedBy int64, subjectID *int64, tanggal time.Time) (Session, error) {
	return f.createFn(ctx, classID, recordedBy, subjectID, tanggal)
}

func (f *fakeRepository) GetSession(ctx context.Context, id int64) (Session, error) {
	return f.getFn(ctx, id)
}

func (f *fakeRepository) ListAttendances(ctx context.Context, sessionID, classID int64) ([]Attendance, error) {
	return f.listFn(ctx, sessionID, classID)
}

func (f *fakeRepository) CountEnrolledStudents(ctx context.Context, classID int64, studentIDs []int64) (int64, error) {
	return f.countFn(ctx, classID, studentIDs)
}

func (f *fakeRepository) RecordAttendances(ctx context.Context, sessionID int64, attendances []AttendanceInput) error {
	return f.recordFn(ctx, sessionID, attendances)
}

func (f *fakeRepository) LockSession(ctx context.Context, id int64) (Session, error) {
	return f.lockFn(ctx, id)
}

func TestCreateSessionValidatesInputBeforeRepository(t *testing.T) {
	// All repo funcs are nil: invalid input must fail first.
	svc := NewService(&fakeRepository{})
	tests := []CreateSessionInput{
		{ClassID: 0, Tanggal: "2026-10-24"},
		{ClassID: 1, ClassSubjectID: ptr(int64(0)), Tanggal: "2026-10-24"},
		{ClassID: 1, Tanggal: "24-10-2026"},
	}
	for _, in := range tests {
		if _, err := svc.CreateSession(context.Background(), Actor{UserID: 1}, in); !errors.Is(err, ErrInvalidClass) &&
			!errors.Is(err, ErrInvalidSubject) && !errors.Is(err, ErrInvalidDate) {
			t.Errorf("CreateSession(%+v) error = %v, want a validation error", in, err)
		}
	}
}

func TestCreateSessionRequiresLinkedStaff(t *testing.T) {
	svc := NewService(&fakeRepository{
		staffFn: func(context.Context, int64) (int64, error) { return 0, pgx.ErrNoRows },
	})

	_, err := svc.CreateSession(context.Background(), Actor{UserID: 9}, CreateSessionInput{ClassID: 1, Tanggal: "2026-10-24"})
	if !errors.Is(err, ErrNotStaff) {
		t.Fatalf("error = %v, want ErrNotStaff", err)
	}
}

func TestCreateSessionEnforcesTeacherOwnership(t *testing.T) {
	svc := NewService(&fakeRepository{
		staffFn: func(context.Context, int64) (int64, error) { return 5, nil },
		canFn:   func(context.Context, int64, int64, *int64) (bool, error) { return false, nil },
	})

	_, err := svc.CreateSession(context.Background(), Actor{UserID: 1}, CreateSessionInput{ClassID: 2, Tanggal: "2026-10-24"})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("error = %v, want ErrForbidden", err)
	}
}

func TestCreateSessionManageBypassesOwnership(t *testing.T) {
	var gotStaffID int64
	svc := NewService(&fakeRepository{
		staffFn: func(context.Context, int64) (int64, error) { return 5, nil },
		canFn: func(context.Context, int64, int64, *int64) (bool, error) {
			t.Fatal("CanRecord must not be called for manage roles")
			return false, nil
		},
		createFn: func(_ context.Context, classID, recordedBy int64, subjectID *int64, tanggal time.Time) (Session, error) {
			gotStaffID = recordedBy
			return Session{ID: 1, ClassID: classID, RecordedBy: recordedBy, Tanggal: Date(tanggal)}, nil
		},
	})

	created, err := svc.CreateSession(context.Background(), Actor{UserID: 1, Manage: true}, CreateSessionInput{ClassID: 2, Tanggal: "2026-10-24"})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if gotStaffID != 5 || created.ClassID != 2 {
		t.Errorf("recorded_by/class_id = %d/%d, want 5/2", gotStaffID, created.ClassID)
	}
	if created.Tanggal.Time().Format(dateLayout) != "2026-10-24" {
		t.Errorf("tanggal = %v, want 2026-10-24", created.Tanggal)
	}
}

func TestCreateSessionPassesSubject(t *testing.T) {
	subjectID := int64(7)
	var got *int64
	svc := NewService(&fakeRepository{
		staffFn: func(context.Context, int64) (int64, error) { return 5, nil },
		canFn:   func(context.Context, int64, int64, *int64) (bool, error) { return true, nil },
		createFn: func(_ context.Context, classID, recordedBy int64, subject *int64, tanggal time.Time) (Session, error) {
			got = subject
			return Session{ID: 1}, nil
		},
	})

	if _, err := svc.CreateSession(context.Background(), Actor{UserID: 1}, CreateSessionInput{ClassID: 2, ClassSubjectID: &subjectID, Tanggal: "2026-10-24"}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if got == nil || *got != subjectID {
		t.Errorf("subject = %v, want %d", got, subjectID)
	}
}

func TestRecordValidatesEntries(t *testing.T) {
	svc := NewService(&fakeRepository{})
	tests := [][]AttendanceInput{
		nil,
		{{StudentID: 1, Status: "bolos"}},
		{{StudentID: 0, Status: "hadir"}},
		{{StudentID: 1, Status: "hadir"}, {StudentID: 1, Status: "izin"}},
	}
	for _, entries := range tests {
		_, err := svc.Record(context.Background(), Actor{UserID: 1}, RecordInput{SessionID: 1, Attendances: entries})
		if !errors.Is(err, ErrEmptyAttendances) && !errors.Is(err, ErrInvalidStatus) &&
			!errors.Is(err, ErrInvalidStudent) && !errors.Is(err, ErrDuplicateStudent) {
			t.Errorf("Record(%+v) error = %v, want a validation error", entries, err)
		}
	}
}

func TestRecordMapsNotFoundAndLocked(t *testing.T) {
	svc := NewService(&fakeRepository{
		staffFn: func(context.Context, int64) (int64, error) { return 5, nil },
		getFn:   func(context.Context, int64) (Session, error) { return Session{}, pgx.ErrNoRows },
	})
	in := RecordInput{SessionID: 1, Attendances: []AttendanceInput{{StudentID: 1, Status: "hadir"}}}
	if _, err := svc.Record(context.Background(), Actor{UserID: 1}, in); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing session error = %v, want ErrNotFound", err)
	}

	svc = NewService(&fakeRepository{
		staffFn: func(context.Context, int64) (int64, error) { return 5, nil },
		getFn:   func(context.Context, int64) (Session, error) { return Session{ID: 1, IsLocked: true}, nil },
		canFn:   func(context.Context, int64, int64, *int64) (bool, error) { return true, nil },
	})
	if _, err := svc.Record(context.Background(), Actor{UserID: 1}, in); !errors.Is(err, ErrLocked) {
		t.Errorf("locked session error = %v, want ErrLocked", err)
	}
}

func TestRecordRejectsUnenrolledStudent(t *testing.T) {
	svc := NewService(&fakeRepository{
		staffFn: func(context.Context, int64) (int64, error) { return 5, nil },
		getFn:   func(context.Context, int64) (Session, error) { return Session{ID: 1, ClassID: 2}, nil },
		canFn:   func(context.Context, int64, int64, *int64) (bool, error) { return true, nil },
		countFn: func(context.Context, int64, []int64) (int64, error) { return 0, nil },
	})

	_, err := svc.Record(context.Background(), Actor{UserID: 1}, RecordInput{
		SessionID:   1,
		Attendances: []AttendanceInput{{StudentID: 1, Status: "hadir"}},
	})
	if !errors.Is(err, ErrStudentNotEnrolled) {
		t.Fatalf("error = %v, want ErrStudentNotEnrolled", err)
	}
}

func TestRecordStoresAndReturnsRoster(t *testing.T) {
	var stored []AttendanceInput
	svc := NewService(&fakeRepository{
		staffFn: func(context.Context, int64) (int64, error) { return 5, nil },
		getFn:   func(context.Context, int64) (Session, error) { return Session{ID: 1, ClassID: 2}, nil },
		canFn:   func(context.Context, int64, int64, *int64) (bool, error) { return true, nil },
		countFn: func(context.Context, int64, []int64) (int64, error) { return 1, nil },
		recordFn: func(_ context.Context, _ int64, entries []AttendanceInput) error {
			stored = entries
			return nil
		},
		listFn: func(context.Context, int64, int64) ([]Attendance, error) {
			status := "hadir"
			return []Attendance{{StudentID: 1, Nama: "Aisyah", Status: &status}}, nil
		},
	})

	detail, err := svc.Record(context.Background(), Actor{UserID: 1}, RecordInput{
		SessionID:   1,
		Attendances: []AttendanceInput{{StudentID: 1, Status: "hadir"}},
	})
	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	if len(stored) != 1 || stored[0].Status != "hadir" {
		t.Errorf("stored = %+v, want one hadir entry", stored)
	}
	if len(detail.Attendances) != 1 || detail.Attendances[0].Nama != "Aisyah" {
		t.Errorf("detail = %+v, want Aisyah", detail)
	}
}

func TestLockRequiresOwnership(t *testing.T) {
	svc := NewService(&fakeRepository{
		staffFn: func(context.Context, int64) (int64, error) { return 5, nil },
		getFn:   func(context.Context, int64) (Session, error) { return Session{ID: 1, ClassID: 2}, nil },
		canFn:   func(context.Context, int64, int64, *int64) (bool, error) { return false, nil },
	})

	if _, err := svc.Lock(context.Background(), Actor{UserID: 1}, 1); !errors.Is(err, ErrForbidden) {
		t.Fatalf("error = %v, want ErrForbidden", err)
	}
}

func TestGetSessionReturnsRoster(t *testing.T) {
	svc := NewService(&fakeRepository{
		getFn: func(context.Context, int64) (Session, error) {
			return Session{ID: 1, ClassID: 2, Tanggal: Date(time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC))}, nil
		},
		listFn: func(context.Context, int64, int64) ([]Attendance, error) {
			return []Attendance{{StudentID: 1, Nama: "Aisyah"}}, nil
		},
	})

	detail, err := svc.GetSession(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if len(detail.Attendances) != 1 || detail.Session.ClassID != 2 {
		t.Errorf("detail = %+v, want class 2 with one student", detail)
	}
}

func TestGetSessionMapsNotFound(t *testing.T) {
	svc := NewService(&fakeRepository{
		getFn: func(context.Context, int64) (Session, error) { return Session{}, pgx.ErrNoRows },
	})
	if _, err := svc.GetSession(context.Background(), 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestLockReturnsLockedSession(t *testing.T) {
	svc := NewService(&fakeRepository{
		staffFn: func(context.Context, int64) (int64, error) { return 5, nil },
		getFn:   func(context.Context, int64) (Session, error) { return Session{ID: 1, ClassID: 2}, nil },
		canFn:   func(context.Context, int64, int64, *int64) (bool, error) { return true, nil },
		lockFn: func(context.Context, int64) (Session, error) {
			return Session{ID: 1, ClassID: 2, IsLocked: true}, nil
		},
	})

	locked, err := svc.Lock(context.Background(), Actor{UserID: 1}, 1)
	if err != nil {
		t.Fatalf("Lock() error = %v", err)
	}
	if !locked.IsLocked {
		t.Error("Lock() returned an unlocked session")
	}
}

func ptr[T any](v T) *T { return &v }
