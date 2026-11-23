package discipline

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeDisciplineRepo struct {
	staffIDFn              func(ctx context.Context, userID int64) (int64, error)
	classInSchoolFn        func(ctx context.Context, classID, schoolID int64) (bool, error)
	studentInSchoolFn      func(ctx context.Context, studentID, schoolID int64) (bool, error)
	listViolationTypesFn   func(ctx context.Context, schoolID int64, includeInactive bool) ([]ViolationType, error)
	getViolationTypeByIDFn func(ctx context.Context, id, schoolID int64) (ViolationType, error)
	createViolationTypeFn  func(ctx context.Context, schoolID int64, in UpsertViolationTypeInput, isActive bool) (ViolationType, error)
	updateViolationTypeFn  func(ctx context.Context, schoolID, id int64, in UpsertViolationTypeInput, isActive bool) (ViolationType, error)
	deleteViolationTypeFn  func(ctx context.Context, schoolID, id int64) error
	recordViolationFn      func(ctx context.Context, schoolID int64, in RecordViolationInput, poin int32, tanggal time.Time) (StudentViolation, error)
	listViolationsFn       func(ctx context.Context, schoolID int64, filter ListViolationsFilter) ([]StudentViolation, error)
	updateViolationFn      func(ctx context.Context, schoolID, id int64, in UpdateViolationInput, poin int32, tanggal time.Time) (StudentViolation, error)
	getStudentSummaryFn    func(ctx context.Context, schoolID, studentID int64) (StudentSummary, error)
	updateFollowupFn       func(ctx context.Context, schoolID, id int64, followup string) (StudentViolation, error)
	deleteViolationFn      func(ctx context.Context, schoolID, id int64) error
	createInspectionFn     func(ctx context.Context, schoolID, inspectorID int64, in CreateInspectionInput, tanggal time.Time, defaultPoints map[int64]int32) (DisciplineInspection, error)
	listInspectionsFn      func(ctx context.Context, schoolID int64, filter ListInspectionsFilter) ([]DisciplineInspection, error)
	listClassStudentsFn    func(ctx context.Context, schoolID, classID int64) ([]ClassStudent, error)
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

func (f *fakeDisciplineRepo) StudentInSchool(ctx context.Context, studentID, schoolID int64) (bool, error) {
	if f.studentInSchoolFn != nil {
		return f.studentInSchoolFn(ctx, studentID, schoolID)
	}
	return true, nil
}

func (f *fakeDisciplineRepo) ListViolationTypes(ctx context.Context, schoolID int64, includeInactive bool) ([]ViolationType, error) {
	if f.listViolationTypesFn != nil {
		return f.listViolationTypesFn(ctx, schoolID, includeInactive)
	}
	return nil, nil
}

func (f *fakeDisciplineRepo) GetViolationTypeByID(ctx context.Context, id, schoolID int64) (ViolationType, error) {
	if f.getViolationTypeByIDFn != nil {
		return f.getViolationTypeByIDFn(ctx, id, schoolID)
	}
	return ViolationType{ID: id, SchoolID: schoolID, Kode: "T01", Nama: "Terlambat", PoinDefault: 2}, nil
}

func (f *fakeDisciplineRepo) CreateViolationType(ctx context.Context, schoolID int64, in UpsertViolationTypeInput, isActive bool) (ViolationType, error) {
	if f.createViolationTypeFn != nil {
		return f.createViolationTypeFn(ctx, schoolID, in, isActive)
	}
	return ViolationType{ID: 1, SchoolID: schoolID, Kode: in.Kode, Nama: in.Nama, Kategori: in.Kategori, PoinDefault: in.PoinDefault, IsActive: isActive}, nil
}

func (f *fakeDisciplineRepo) UpdateViolationType(ctx context.Context, schoolID, id int64, in UpsertViolationTypeInput, isActive bool) (ViolationType, error) {
	if f.updateViolationTypeFn != nil {
		return f.updateViolationTypeFn(ctx, schoolID, id, in, isActive)
	}
	return ViolationType{ID: id, SchoolID: schoolID, Kode: in.Kode, Nama: in.Nama, Kategori: in.Kategori, PoinDefault: in.PoinDefault, IsActive: isActive}, nil
}

func (f *fakeDisciplineRepo) DeleteViolationType(ctx context.Context, schoolID, id int64) error {
	if f.deleteViolationTypeFn != nil {
		return f.deleteViolationTypeFn(ctx, schoolID, id)
	}
	return nil
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

func (f *fakeDisciplineRepo) UpdateViolation(ctx context.Context, schoolID, id int64, in UpdateViolationInput, poin int32, _ time.Time) (StudentViolation, error) {
	if f.updateViolationFn != nil {
		return f.updateViolationFn(ctx, schoolID, id, in, poin, time.Time{})
	}
	return StudentViolation{ID: id, SchoolID: schoolID, ViolationTypeID: in.ViolationTypeID, Poin: poin}, nil
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

func (f *fakeDisciplineRepo) DeleteViolation(ctx context.Context, schoolID, id int64) error {
	if f.deleteViolationFn != nil {
		return f.deleteViolationFn(ctx, schoolID, id)
	}
	return nil
}

func (f *fakeDisciplineRepo) ListSchoolStudents(context.Context, int64, *int64) ([]SchoolStudent, error) {
	return []SchoolStudent{}, nil
}

func (f *fakeDisciplineRepo) ListWatchlist(context.Context, int64, int64) ([]WatchlistStudent, error) {
	return []WatchlistStudent{}, nil
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

func (f *fakeDisciplineRepo) UpdateInspection(_ context.Context, schoolID, id int64, in UpdateInspectionInput, _ time.Time, _ map[int64]int32) (DisciplineInspection, error) {
	return DisciplineInspection{ID: id, SchoolID: schoolID, JumlahDiperiksa: in.JumlahDiperiksa, Catatan: in.Catatan}, nil
}

func (f *fakeDisciplineRepo) DeleteInspection(context.Context, int64, int64) error {
	return nil
}

func (f *fakeDisciplineRepo) ListClassStudents(ctx context.Context, schoolID, classID int64) ([]ClassStudent, error) {
	if f.listClassStudentsFn != nil {
		return f.listClassStudentsFn(ctx, schoolID, classID)
	}
	return nil, nil
}

func (f *fakeDisciplineRepo) ListCounselingRecords(context.Context, int64, ListCounselingFilter) ([]CounselingRecord, error) {
	return []CounselingRecord{}, nil
}

func (f *fakeDisciplineRepo) CreateCounselingRecord(_ context.Context, schoolID int64, counselorID *int64, in CreateCounselingInput, _ time.Time) (CounselingRecord, error) {
	return CounselingRecord{ID: 1, SchoolID: schoolID, StudentID: in.StudentID, CounselorID: counselorID, Topik: in.Topik, Status: in.Status}, nil
}

func (f *fakeDisciplineRepo) UpdateCounselingRecord(_ context.Context, schoolID, id int64, in UpdateCounselingInput, _ time.Time) (CounselingRecord, error) {
	return CounselingRecord{ID: id, SchoolID: schoolID, Topik: in.Topik, Status: in.Status}, nil
}

func (f *fakeDisciplineRepo) DeleteCounselingRecord(context.Context, int64, int64) error {
	return nil
}

func (f *fakeDisciplineRepo) ListParentSummons(context.Context, int64, ListSummonsFilter) ([]ParentSummons, error) {
	return []ParentSummons{}, nil
}

func (f *fakeDisciplineRepo) CreateParentSummons(_ context.Context, schoolID int64, createdBy *int64, in CreateParentSummonsInput, _ time.Time, jadwal time.Time) (ParentSummons, error) {
	return ParentSummons{ID: 1, SchoolID: schoolID, StudentID: in.StudentID, CreatedBy: createdBy, NomorSurat: in.NomorSurat, JadwalPertemuan: jadwal, Ruangan: in.Ruangan, Alasan: in.Alasan, Status: in.Status}, nil
}

func (f *fakeDisciplineRepo) UpdateParentSummons(_ context.Context, schoolID, id int64, in UpdateParentSummonsInput, _ time.Time, jadwal time.Time) (ParentSummons, error) {
	return ParentSummons{ID: id, SchoolID: schoolID, NomorSurat: in.NomorSurat, JadwalPertemuan: jadwal, Ruangan: in.Ruangan, Alasan: in.Alasan}, nil
}

func (f *fakeDisciplineRepo) UpdateParentSummonsStatus(_ context.Context, schoolID, id int64, status string, catatan *string) (ParentSummons, error) {
	return ParentSummons{ID: id, SchoolID: schoolID, Status: status, CatatanHasil: catatan}, nil
}

func (f *fakeDisciplineRepo) DeleteParentSummons(context.Context, int64, int64) error {
	return nil
}

func TestListViolationTypes_RequiresSchool(t *testing.T) {
	svc := NewService(&fakeDisciplineRepo{})
	_, err := svc.ListViolationTypes(context.Background(), Actor{SchoolID: 0, Roles: []string{"kesiswaan"}}, false)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestDisciplineService_RequiresKesiswaanRole(t *testing.T) {
	svc := NewService(&fakeDisciplineRepo{})
	ctx := context.Background()

	for _, role := range []string{"guru", "admin", "tu", "kepala_sekolah", ""} {
		var roles []string
		if role != "" {
			roles = []string{role}
		}
		actor := Actor{UserID: 1, SchoolID: 1, Roles: roles}

		if _, err := svc.ListViolationTypes(ctx, actor, false); !errors.Is(err, ErrForbidden) {
			t.Errorf("role %q ListViolationTypes: expected ErrForbidden, got %v", role, err)
		}
		if _, err := svc.ListViolations(ctx, actor, ListViolationsFilter{}); !errors.Is(err, ErrForbidden) {
			t.Errorf("role %q ListViolations: expected ErrForbidden, got %v", role, err)
		}
		if _, err := svc.RecordViolation(ctx, actor, RecordViolationInput{StudentID: 1, ViolationTypeID: 1, Tanggal: "2026-10-24"}); !errors.Is(err, ErrForbidden) {
			t.Errorf("role %q RecordViolation: expected ErrForbidden, got %v", role, err)
		}
		if _, err := svc.UpdateFollowup(ctx, actor, 1, "test"); !errors.Is(err, ErrForbidden) {
			t.Errorf("role %q UpdateFollowup: expected ErrForbidden, got %v", role, err)
		}
		if _, err := svc.CreateInspection(ctx, actor, CreateInspectionInput{ClassID: 1, Tanggal: "2026-10-24"}); !errors.Is(err, ErrForbidden) {
			t.Errorf("role %q CreateInspection: expected ErrForbidden, got %v", role, err)
		}
		if _, err := svc.ListInspections(ctx, actor, ListInspectionsFilter{}); !errors.Is(err, ErrForbidden) {
			t.Errorf("role %q ListInspections: expected ErrForbidden, got %v", role, err)
		}
		if _, err := svc.GetStudentSummary(ctx, actor, 1); !errors.Is(err, ErrForbidden) {
			t.Errorf("role %q GetStudentSummary: expected ErrForbidden, got %v", role, err)
		}
		if _, err := svc.ListClassStudents(ctx, actor, 1); !errors.Is(err, ErrForbidden) {
			t.Errorf("role %q ListClassStudents: expected ErrForbidden, got %v", role, err)
		}
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

func TestViolationTypeAndSummonsValidation(t *testing.T) {
	svc := NewService(&fakeDisciplineRepo{})
	ctx := context.Background()
	actor := Actor{UserID: 1, SchoolID: 1, Roles: []string{"kesiswaan"}}

	if _, err := svc.CreateViolationType(ctx, actor, UpsertViolationTypeInput{
		Kode: "ATR-99", Nama: "Test", Kategori: "invalid", PoinDefault: 5,
	}); !errors.Is(err, ErrInvalidCategory) {
		t.Errorf("expected ErrInvalidCategory, got %v", err)
	}

	vt, err := svc.CreateViolationType(ctx, actor, UpsertViolationTypeInput{
		Kode: "atr-99", Nama: "Seragam Pramuka", Kategori: "atribut", PoinDefault: 3,
	})
	if err != nil {
		t.Fatalf("unexpected CreateViolationType err: %v", err)
	}
	if vt.Kode != "ATR-99" {
		t.Errorf("expected uppercased kode ATR-99, got %q", vt.Kode)
	}

	if _, err := svc.CreateParentSummons(ctx, actor, CreateParentSummonsInput{
		StudentID: 1, NomorSurat: "001/SD-BA", TanggalSurat: "2026-11-23", JadwalPertemuan: "2026-11-25T09:00",
		Ruangan: "Ruang BK", Alasan: "Pembinaan", Status: "invalid",
	}); !errors.Is(err, ErrInvalidStatus) {
		t.Errorf("expected ErrInvalidStatus, got %v", err)
	}

	if _, err := svc.CreateParentSummons(ctx, actor, CreateParentSummonsInput{
		StudentID: 1, NomorSurat: "001/SD-BA", TanggalSurat: "2026-11-23", JadwalPertemuan: "2026-11-25T09:00",
		Ruangan: "Ruang BK", Alasan: "Pembinaan",
	}); err != nil {
		t.Fatalf("unexpected CreateParentSummons err: %v", err)
	}
}
