package discipline

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeDisciplineRepo struct {
	staffIDFn           func(ctx context.Context, userID int64) (int64, error)
	classInSchoolFn     func(ctx context.Context, classID, schoolID int64) (bool, error)
	listViolationTypesFn func(ctx context.Context, schoolID int64) ([]ViolationType, error)
	getViolationTypeByIDFn func(ctx context.Context, id, schoolID int64) (ViolationType, error)
	recordViolationFn   func(ctx context.Context, schoolID int64, in RecordViolationInput, poin int32, tanggal time.Time) (StudentViolation, error)
	listViolationsFn    func(ctx context.Context, schoolID int64, filter ListViolationsFilter) ([]StudentViolation, error)
	getStudentSummaryFn func(ctx context.Context, schoolID, studentID int64) (StudentSummary, error)
	updateFollowupFn    func(ctx context.Context, schoolID, id int64, followup string) (StudentViolation, error)
	createInspectionFn  func(ctx context.Context, schoolID, inspectorID int64, in CreateInspectionInput, tanggal time.Time, defaultPoints map[int64]int32) (DisciplineInspection, error)
	listInspectionsFn   func(ctx context.Context, schoolID int64, filter ListInspectionsFilter) ([]DisciplineInspection, error)
	listClassStudentsFn func(ctx context.Context, schoolID, classID int64) ([]ClassStudent, error)
}

func (f *fakeDisciplineRepo) StaffIDByUserID(ctx context.Context, userID int64) (int64, error) {
	if f.staffIDFn != nil {
		return f.staffIDFn(ctx, userID)
	}
	return 1, nil
}

func (f *fakeDisciplineRepo) ClassInSchool(ctx context.Context, classID, schoolID int64) (bool, error) {
	if f.classInSchoolFn != nil {
		return f.classInSchoolFn(ctx, classID, schoolID)
	}
	return true, nil
}

func (f *fakeDisciplineRepo) ListViolationTypes(ctx context.Context, schoolID int64) ([]ViolationType, error) {
	if f.listViolationTypesFn != nil {
		return f.listViolationTypesFn(ctx, schoolID)
	}
	return nil, nil
}

func (f *fakeDisciplineRepo) GetViolationTypeByID(ctx context.Context, id, schoolID int64) (ViolationType, error) {
	if f.getViolationTypeByIDFn != nil {
		return f.getViolationTypeByIDFn(ctx, id, schoolID)
	}
	return ViolationType{ID: id, SchoolID: schoolID, Kode: "T01", Nama: "Terlambat", PoinDefault: 2}, nil
}

func (f *fakeDisciplineRepo) RecordViolation(ctx context.Context, schoolID int64, in RecordViolationInput, poin int32, tanggal time.Time) (StudentViolation, error) {
	if f.recordViolationFn != nil {
		return f.recordViolationFn(ctx, schoolID, in, poin, tanggal)
	}
	return StudentViolation{ID: 10, SchoolID: schoolID, StudentID: in.StudentID, Poin: poin}, nil
}

func (f *fakeDisciplineRepo) ListViolations(ctx context.Context, schoolID int64, filter ListViolationsFilter) ([]StudentViolation, error) {
	if f.listViolationsFn != nil {
		return f.listViolationsFn(ctx, schoolID, filter)
	}
	return nil, nil
}

func (f *fakeDisciplineRepo) GetStudentSummary(ctx context.Context, schoolID, studentID int64) (StudentSummary, error) {
	if f.getStudentSummaryFn != nil {
		return f.getStudentSummaryFn(ctx, schoolID, studentID)
	}
	return StudentSummary{StudentID: studentID, TotalPoints: 35, TotalIncidents: 4, StatusSP: CalculateSPStatus(35)}, nil
}

func (f *fakeDisciplineRepo) UpdateFollowup(ctx context.Context, schoolID, id int64, followup string) (StudentViolation, error) {
	if f.updateFollowupFn != nil {
		return f.updateFollowupFn(ctx, schoolID, id, followup)
	}
	return StudentViolation{ID: id, SchoolID: schoolID, TindakLanjut: &followup}, nil
}

func (f *fakeDisciplineRepo) CreateInspectionWithViolations(ctx context.Context, schoolID, inspectorID int64, in CreateInspectionInput, tanggal time.Time, defaultPoints map[int64]int32) (DisciplineInspection, error) {
	if f.createInspectionFn != nil {
		return f.createInspectionFn(ctx, schoolID, inspectorID, in, tanggal, defaultPoints)
	}
	return DisciplineInspection{ID: 1, SchoolID: schoolID, ClassID: in.ClassID, InspectorID: inspectorID}, nil
}

func (f *fakeDisciplineRepo) ListInspections(ctx context.Context, schoolID int64, filter ListInspectionsFilter) ([]DisciplineInspection, error) {
	if f.listInspectionsFn != nil {
		return f.listInspectionsFn(ctx, schoolID, filter)
	}
	return nil, nil
}

func (f *fakeDisciplineRepo) ListClassStudents(ctx context.Context, schoolID, classID int64) ([]ClassStudent, error) {
	if f.listClassStudentsFn != nil {
		return f.listClassStudentsFn(ctx, schoolID, classID)
	}
	return nil, nil
}

func TestListViolationTypes_RequiresSchool(t *testing.T) {
	svc := NewService(&fakeDisciplineRepo{})
	_, err := svc.ListViolationTypes(context.Background(), Actor{SchoolID: 0})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestRecordViolation_ValidatesInput(t *testing.T) {
	svc := NewService(&fakeDisciplineRepo{})
	ctx := context.Background()
	actor := Actor{UserID: 1, SchoolID: 1, Roles: []string{"kesiswaan"}}

	// Missing student
	_, err := svc.RecordViolation(ctx, actor, RecordViolationInput{StudentID: 0, ViolationTypeID: 1, Tanggal: "2026-10-24"})
	if !errors.Is(err, ErrInvalidStudent) {
		t.Errorf("expected ErrInvalidStudent, got %v", err)
	}

	// Missing violation type
	_, err = svc.RecordViolation(ctx, actor, RecordViolationInput{StudentID: 1, ViolationTypeID: 0, Tanggal: "2026-10-24"})
	if !errors.Is(err, ErrInvalidViolationType) {
		t.Errorf("expected ErrInvalidViolationType, got %v", err)
	}

	// Invalid date
	_, err = svc.RecordViolation(ctx, actor, RecordViolationInput{StudentID: 1, ViolationTypeID: 1, Tanggal: "invalid-date"})
	if !errors.Is(err, ErrInvalidDate) {
		t.Errorf("expected ErrInvalidDate, got %v", err)
	}

	// Negative points
	negPoint := int32(-5)
	_, err = svc.RecordViolation(ctx, actor, RecordViolationInput{StudentID: 1, ViolationTypeID: 1, Tanggal: "2026-10-24", Poin: &negPoint})
	if !errors.Is(err, ErrInvalidPoints) {
		t.Errorf("expected ErrInvalidPoints, got %v", err)
	}

	// Success with default points
	res, err := svc.RecordViolation(ctx, actor, RecordViolationInput{StudentID: 1, ViolationTypeID: 1, Tanggal: "2026-10-24"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Poin != 2 {
		t.Errorf("expected default point 2, got %d", res.Poin)
	}
}

func TestCreateInspection_ValidatesClassAndStaff(t *testing.T) {
	repo := &fakeDisciplineRepo{
		classInSchoolFn: func(ctx context.Context, classID, schoolID int64) (bool, error) {
			return false, nil
		},
	}
	svc := NewService(repo)
	ctx := context.Background()
	actor := Actor{UserID: 1, SchoolID: 1, Roles: []string{"kesiswaan"}}

	// Class not in school
	_, err := svc.CreateInspection(ctx, actor, CreateInspectionInput{
		ClassID:         999,
		Tanggal:         "2026-10-24",
		JumlahDiperiksa: 30,
	})
	if !errors.Is(err, ErrInvalidClass) {
		t.Errorf("expected ErrInvalidClass, got %v", err)
	}
}

func TestCalculateSPStatus(t *testing.T) {
	cases := []struct {
		points   int64
		expected string
	}{
		{0, "Aman"},
		{10, "Aman"},
		{15, "Peringatan Lisan / Pembinaan Wali Kelas"},
		{30, "Panggilan Orang Tua"},
		{45, "Panggilan Orang Tua"},
		{50, "SP1 (Surat Peringatan 1)"},
		{75, "SP2 (Skorsing 3 Hari)"},
		{100, "SP3 (Dikeluarkan / Pindah Sekolah)"},
	}

	for _, c := range cases {
		got := CalculateSPStatus(c.points)
		if got != c.expected {
			t.Errorf("points %d: expected %q, got %q", c.points, c.expected, got)
		}
	}
}
