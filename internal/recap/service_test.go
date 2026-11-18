package recap

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

type fakeRepository struct {
	generateFn      func(context.Context, int64, int16, int16) error
	listFn          func(context.Context, int64, int16, int16) ([]StudentRecap, error)
	statusFn        func(context.Context, int64, int64) (string, error)
	verifyFn        func(context.Context, int64, int64, int64) (Verification, error)
	staffGenerateFn func(context.Context, int64, int16, int16) error
	staffListFn     func(context.Context, int64, int16, int16) ([]StaffRecap, error)
	staffStatusFn   func(context.Context, int64, int64) (string, error)
	staffVerifyFn   func(context.Context, int64, int64, int64) (Verification, error)
}

func (f fakeRepository) GenerateStudentRecaps(ctx context.Context, schoolID int64, month, year int16) error {
	return f.generateFn(ctx, schoolID, month, year)
}

func (f fakeRepository) ListStudentRecaps(ctx context.Context, schoolID int64, month, year int16) ([]StudentRecap, error) {
	return f.listFn(ctx, schoolID, month, year)
}

func (f fakeRepository) StudentRecapStatus(ctx context.Context, id, schoolID int64) (string, error) {
	return f.statusFn(ctx, id, schoolID)
}

func (f fakeRepository) VerifyStudentRecap(ctx context.Context, id, schoolID, userID int64) (Verification, error) {
	return f.verifyFn(ctx, id, schoolID, userID)
}

func (f fakeRepository) GenerateStaffRecaps(ctx context.Context, schoolID int64, month, year int16) error {
	return f.staffGenerateFn(ctx, schoolID, month, year)
}

func (f fakeRepository) ListStaffRecaps(ctx context.Context, schoolID int64, month, year int16) ([]StaffRecap, error) {
	return f.staffListFn(ctx, schoolID, month, year)
}

func (f fakeRepository) StaffRecapStatus(ctx context.Context, id, schoolID int64) (string, error) {
	return f.staffStatusFn(ctx, id, schoolID)
}

func (f fakeRepository) VerifyStaffRecap(ctx context.Context, id, schoolID, userID int64) (Verification, error) {
	return f.staffVerifyFn(ctx, id, schoolID, userID)
}

func TestGenerateStudentRecapsValidatesPeriodBeforeRepository(t *testing.T) {
	svc := NewService(fakeRepository{})
	tests := []struct {
		name   string
		school int64
		month  int
		year   int
		want   error
	}{
		{name: "missing school", school: 0, month: 10, year: 2026, want: ErrForbidden},
		{name: "invalid month", school: 1, month: 13, year: 2026, want: ErrInvalidPeriod},
		{name: "invalid year", school: 1, month: 10, year: 0, want: ErrInvalidPeriod},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.GenerateStudentRecaps(context.Background(), tt.school, tt.month, tt.year)
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestGenerateStudentRecapsUsesSchoolAndReturnsList(t *testing.T) {
	var gotSchool int64
	var gotMonth, gotYear int16
	want := []StudentRecap{{ID: 8, StudentID: 2, Month: 10, Year: 2026, Status: "draft"}}
	svc := NewService(fakeRepository{
		generateFn: func(_ context.Context, schoolID int64, month, year int16) error {
			gotSchool, gotMonth, gotYear = schoolID, month, year
			return nil
		},
		listFn: func(_ context.Context, schoolID int64, month, year int16) ([]StudentRecap, error) {
			if schoolID != gotSchool || month != gotMonth || year != gotYear {
				t.Fatalf("list scope = %d/%d/%d, generation scope = %d/%d/%d", schoolID, month, year, gotSchool, gotMonth, gotYear)
			}
			return want, nil
		},
	})

	got, err := svc.GenerateStudentRecaps(context.Background(), 12, 10, 2026)
	if err != nil {
		t.Fatalf("GenerateStudentRecaps() error = %v", err)
	}
	if gotSchool != 12 || gotMonth != 10 || gotYear != 2026 || len(got) != 1 || got[0].ID != want[0].ID {
		t.Fatalf("generation/result = %d/%d/%d, %+v", gotSchool, gotMonth, gotYear, got)
	}
}

func TestVerifyStudentRecapMapsLockedAndOutOfTenant(t *testing.T) {
	tests := []struct {
		name      string
		status    string
		statusErr error
		want      error
	}{
		{name: "already locked", status: "dikunci", want: ErrLocked},
		{name: "not in tenant", statusErr: pgx.ErrNoRows, want: ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(fakeRepository{
				verifyFn: func(context.Context, int64, int64, int64) (Verification, error) {
					return Verification{}, pgx.ErrNoRows
				},
				statusFn: func(context.Context, int64, int64) (string, error) {
					return tt.status, tt.statusErr
				},
			})
			_, err := svc.VerifyStudentRecap(context.Background(), 8, 12, 4)
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestGenerateStaffRecapsUsesSchoolAndReturnsList(t *testing.T) {
	var gotSchool int64
	var gotMonth, gotYear int16
	want := []StaffRecap{{ID: 9, StaffID: 2, Month: 10, Year: 2026, Status: "draft"}}
	svc := NewService(fakeRepository{
		staffGenerateFn: func(_ context.Context, schoolID int64, month, year int16) error {
			gotSchool, gotMonth, gotYear = schoolID, month, year
			return nil
		},
		staffListFn: func(_ context.Context, schoolID int64, month, year int16) ([]StaffRecap, error) {
			if schoolID != gotSchool || month != gotMonth || year != gotYear {
				t.Fatalf("list scope = %d/%d/%d, generation scope = %d/%d/%d", schoolID, month, year, gotSchool, gotMonth, gotYear)
			}
			return want, nil
		},
	})

	got, err := svc.GenerateStaffRecaps(context.Background(), 12, 10, 2026)
	if err != nil {
		t.Fatalf("GenerateStaffRecaps() error = %v", err)
	}
	if gotSchool != 12 || gotMonth != 10 || gotYear != 2026 || len(got) != 1 || got[0].ID != want[0].ID {
		t.Fatalf("generation/result = %d/%d/%d, %+v", gotSchool, gotMonth, gotYear, got)
	}
}
