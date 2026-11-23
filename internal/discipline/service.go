package discipline

import (
	"context"
	"fmt"
	"strings"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) authorize(actor Actor) error {
	if actor.SchoolID <= 0 || !actor.IsKesiswaan() {
		return ErrForbidden
	}
	return nil
}

func validCategory(kat string) bool {
	switch kat {
	case "atribut", "kerapihan", "perilaku", "lainnya":
		return true
	default:
		return false
	}
}

func validCounselingStatus(status string) bool {
	switch status {
	case "scheduled", "ongoing", "completed":
		return true
	default:
		return false
	}
}

func validSummonsStatus(status string) bool {
	switch status {
	case "draft", "sent", "completed":
		return true
	default:
		return false
	}
}

func (s *Service) ListViolationTypes(ctx context.Context, actor Actor, includeInactive bool) ([]ViolationType, error) {
	if err := s.authorize(actor); err != nil {
		return nil, err
	}
	return s.repo.ListViolationTypes(ctx, actor.SchoolID, includeInactive)
}

func (s *Service) CreateViolationType(ctx context.Context, actor Actor, in UpsertViolationTypeInput) (ViolationType, error) {
	if err := s.authorize(actor); err != nil {
		return ViolationType{}, err
	}
	in.Kode = strings.ToUpper(strings.TrimSpace(in.Kode))
	in.Nama = strings.TrimSpace(in.Nama)
	in.Kategori = strings.ToLower(strings.TrimSpace(in.Kategori))
	if in.Kode == "" || in.Nama == "" {
		return ViolationType{}, ErrInvalidPayload
	}
	if !validCategory(in.Kategori) {
		return ViolationType{}, ErrInvalidCategory
	}
	if in.PoinDefault < 0 {
		return ViolationType{}, ErrInvalidPoints
	}
	isActive := true
	if in.IsActive != nil {
		isActive = *in.IsActive
	}
	return s.repo.CreateViolationType(ctx, actor.SchoolID, in, isActive)
}

func (s *Service) UpdateViolationType(ctx context.Context, actor Actor, id int64, in UpsertViolationTypeInput) (ViolationType, error) {
	if err := s.authorize(actor); err != nil {
		return ViolationType{}, err
	}
	if id <= 0 {
		return ViolationType{}, ErrNotFound
	}
	in.Kode = strings.ToUpper(strings.TrimSpace(in.Kode))
	in.Nama = strings.TrimSpace(in.Nama)
	in.Kategori = strings.ToLower(strings.TrimSpace(in.Kategori))
	if in.Kode == "" || in.Nama == "" {
		return ViolationType{}, ErrInvalidPayload
	}
	if !validCategory(in.Kategori) {
		return ViolationType{}, ErrInvalidCategory
	}
	if in.PoinDefault < 0 {
		return ViolationType{}, ErrInvalidPoints
	}
	isActive := true
	if in.IsActive != nil {
		isActive = *in.IsActive
	}
	return s.repo.UpdateViolationType(ctx, actor.SchoolID, id, in, isActive)
}

func (s *Service) DeleteViolationType(ctx context.Context, actor Actor, id int64) error {
	if err := s.authorize(actor); err != nil {
		return err
	}
	if id <= 0 {
		return ErrNotFound
	}
	return s.repo.DeleteViolationType(ctx, actor.SchoolID, id)
}

func (s *Service) RecordViolation(ctx context.Context, actor Actor, in RecordViolationInput) (StudentViolation, error) {
	if err := s.authorize(actor); err != nil {
		return StudentViolation{}, err
	}
	if in.StudentID <= 0 {
		return StudentViolation{}, ErrInvalidStudent
	}
	if in.ViolationTypeID <= 0 {
		return StudentViolation{}, ErrInvalidViolationType
	}

	tanggal, err := ParseDate(in.Tanggal)
	if err != nil {
		return StudentViolation{}, err
	}

	okStudent, err := s.repo.StudentInSchool(ctx, in.StudentID, actor.SchoolID)
	if err != nil {
		return StudentViolation{}, fmt.Errorf("verify student: %w", err)
	}
	if !okStudent {
		return StudentViolation{}, ErrInvalidStudent
	}

	vt, err := s.repo.GetViolationTypeByID(ctx, in.ViolationTypeID, actor.SchoolID)
	if err != nil {
		return StudentViolation{}, ErrInvalidViolationType
	}

	poin := vt.PoinDefault
	if in.Poin != nil {
		if *in.Poin < 0 {
			return StudentViolation{}, ErrInvalidPoints
		}
		poin = *in.Poin
	}

	return s.repo.RecordViolation(ctx, actor.SchoolID, in, poin, tanggal.Time())
}

func (s *Service) ListViolations(ctx context.Context, actor Actor, filter ListViolationsFilter) ([]StudentViolation, error) {
	if err := s.authorize(actor); err != nil {
		return nil, err
	}
	return s.repo.ListViolations(ctx, actor.SchoolID, filter)
}

func (s *Service) UpdateViolation(ctx context.Context, actor Actor, id int64, in UpdateViolationInput) (StudentViolation, error) {
	if err := s.authorize(actor); err != nil {
		return StudentViolation{}, err
	}
	if id <= 0 {
		return StudentViolation{}, ErrNotFound
	}
	if in.ViolationTypeID <= 0 {
		return StudentViolation{}, ErrInvalidViolationType
	}
	tanggal, err := ParseDate(in.Tanggal)
	if err != nil {
		return StudentViolation{}, err
	}
	vt, err := s.repo.GetViolationTypeByID(ctx, in.ViolationTypeID, actor.SchoolID)
	if err != nil {
		return StudentViolation{}, ErrInvalidViolationType
	}
	poin := vt.PoinDefault
	if in.Poin != nil {
		if *in.Poin < 0 {
			return StudentViolation{}, ErrInvalidPoints
		}
		poin = *in.Poin
	}
	return s.repo.UpdateViolation(ctx, actor.SchoolID, id, in, poin, tanggal.Time())
}

func (s *Service) UpdateFollowup(ctx context.Context, actor Actor, id int64, followup string) (StudentViolation, error) {
	if err := s.authorize(actor); err != nil {
		return StudentViolation{}, err
	}
	if id <= 0 {
		return StudentViolation{}, ErrNotFound
	}
	return s.repo.UpdateFollowup(ctx, actor.SchoolID, id, followup)
}

func (s *Service) DeleteViolation(ctx context.Context, actor Actor, id int64) error {
	if err := s.authorize(actor); err != nil {
		return err
	}
	if id <= 0 {
		return ErrNotFound
	}
	return s.repo.DeleteViolation(ctx, actor.SchoolID, id)
}

func (s *Service) GetStudentSummary(ctx context.Context, actor Actor, studentID int64) (StudentSummary, error) {
	if err := s.authorize(actor); err != nil {
		return StudentSummary{}, err
	}
	if studentID <= 0 {
		return StudentSummary{}, ErrInvalidStudent
	}
	return s.repo.GetStudentSummary(ctx, actor.SchoolID, studentID)
}

func (s *Service) ListSchoolStudents(ctx context.Context, actor Actor, classID *int64) ([]SchoolStudent, error) {
	if err := s.authorize(actor); err != nil {
		return nil, err
	}
	if classID != nil && *classID > 0 {
		classOk, err := s.repo.ClassInSchool(ctx, *classID, actor.SchoolID)
		if err != nil {
			return nil, fmt.Errorf("verify class: %w", err)
		}
		if !classOk {
			return nil, ErrInvalidClass
		}
	}
	return s.repo.ListSchoolStudents(ctx, actor.SchoolID, classID)
}

func (s *Service) ListWatchlist(ctx context.Context, actor Actor, minPoints int64) ([]WatchlistStudent, error) {
	if err := s.authorize(actor); err != nil {
		return nil, err
	}
	if minPoints < 0 {
		minPoints = 0
	}
	return s.repo.ListWatchlist(ctx, actor.SchoolID, minPoints)
}

func (s *Service) CreateInspection(ctx context.Context, actor Actor, in CreateInspectionInput) (DisciplineInspection, error) {
	if err := s.authorize(actor); err != nil {
		return DisciplineInspection{}, err
	}
	if in.ClassID <= 0 {
		return DisciplineInspection{}, ErrInvalidClass
	}
	tanggal, err := ParseDate(in.Tanggal)
	if err != nil {
		return DisciplineInspection{}, err
	}

	staffID, err := s.repo.StaffIDByUserID(ctx, actor.UserID)
	if err != nil {
		return DisciplineInspection{}, ErrNotStaff
	}

	classOk, err := s.repo.ClassInSchool(ctx, in.ClassID, actor.SchoolID)
	if err != nil {
		return DisciplineInspection{}, fmt.Errorf("verify class: %w", err)
	}
	if !classOk {
		return DisciplineInspection{}, ErrInvalidClass
	}

	defaultPoints := make(map[int64]int32)
	for _, v := range in.Violations {
		if v.StudentID <= 0 {
			return DisciplineInspection{}, ErrInvalidStudent
		}
		if v.ViolationTypeID <= 0 {
			return DisciplineInspection{}, ErrInvalidViolationType
		}
		if _, exists := defaultPoints[v.ViolationTypeID]; !exists {
			vt, err := s.repo.GetViolationTypeByID(ctx, v.ViolationTypeID, actor.SchoolID)
			if err != nil {
				return DisciplineInspection{}, ErrInvalidViolationType
			}
			defaultPoints[v.ViolationTypeID] = vt.PoinDefault
		}
	}

	return s.repo.CreateInspectionWithViolations(ctx, actor.SchoolID, staffID, in, tanggal.Time(), defaultPoints)
}

func (s *Service) ListInspections(ctx context.Context, actor Actor, filter ListInspectionsFilter) ([]DisciplineInspection, error) {
	if err := s.authorize(actor); err != nil {
		return nil, err
	}
	return s.repo.ListInspections(ctx, actor.SchoolID, filter)
}

func (s *Service) UpdateInspection(ctx context.Context, actor Actor, id int64, in UpdateInspectionInput) (DisciplineInspection, error) {
	if err := s.authorize(actor); err != nil {
		return DisciplineInspection{}, err
	}
	if id <= 0 {
		return DisciplineInspection{}, ErrNotFound
	}
	tanggal, err := ParseDate(in.Tanggal)
	if err != nil {
		return DisciplineInspection{}, err
	}
	if in.JumlahDiperiksa < 0 {
		return DisciplineInspection{}, ErrInvalidPoints
	}

	defaultPoints := make(map[int64]int32)
	for _, v := range in.Violations {
		if v.StudentID <= 0 {
			return DisciplineInspection{}, ErrInvalidStudent
		}
		if v.ViolationTypeID <= 0 {
			return DisciplineInspection{}, ErrInvalidViolationType
		}
		if _, exists := defaultPoints[v.ViolationTypeID]; !exists {
			vt, err := s.repo.GetViolationTypeByID(ctx, v.ViolationTypeID, actor.SchoolID)
			if err != nil {
				return DisciplineInspection{}, ErrInvalidViolationType
			}
			defaultPoints[v.ViolationTypeID] = vt.PoinDefault
		}
	}

	return s.repo.UpdateInspection(ctx, actor.SchoolID, id, in, tanggal.Time(), defaultPoints)
}

func (s *Service) DeleteInspection(ctx context.Context, actor Actor, id int64) error {
	if err := s.authorize(actor); err != nil {
		return err
	}
	if id <= 0 {
		return ErrNotFound
	}
	return s.repo.DeleteInspection(ctx, actor.SchoolID, id)
}

func (s *Service) ListClassStudents(ctx context.Context, actor Actor, classID int64) ([]ClassStudent, error) {
	if err := s.authorize(actor); err != nil {
		return nil, err
	}
	if classID <= 0 {
		return nil, ErrInvalidClass
	}
	classOk, err := s.repo.ClassInSchool(ctx, classID, actor.SchoolID)
	if err != nil {
		return nil, fmt.Errorf("verify class: %w", err)
	}
	if !classOk {
		return nil, ErrInvalidClass
	}
	return s.repo.ListClassStudents(ctx, actor.SchoolID, classID)
}

func (s *Service) ListCounselingRecords(ctx context.Context, actor Actor, filter ListCounselingFilter) ([]CounselingRecord, error) {
	if err := s.authorize(actor); err != nil {
		return nil, err
	}
	return s.repo.ListCounselingRecords(ctx, actor.SchoolID, filter)
}

func (s *Service) CreateCounselingRecord(ctx context.Context, actor Actor, in CreateCounselingInput) (CounselingRecord, error) {
	if err := s.authorize(actor); err != nil {
		return CounselingRecord{}, err
	}
	if in.StudentID <= 0 {
		return CounselingRecord{}, ErrInvalidStudent
	}
	in.Topik = strings.TrimSpace(in.Topik)
	if in.Topik == "" {
		return CounselingRecord{}, ErrInvalidPayload
	}
	tanggal, err := ParseDate(in.Tanggal)
	if err != nil {
		return CounselingRecord{}, err
	}
	if in.Status == "" {
		in.Status = "scheduled"
	}
	if !validCounselingStatus(in.Status) {
		return CounselingRecord{}, ErrInvalidStatus
	}
	okStudent, err := s.repo.StudentInSchool(ctx, in.StudentID, actor.SchoolID)
	if err != nil {
		return CounselingRecord{}, fmt.Errorf("verify student: %w", err)
	}
	if !okStudent {
		return CounselingRecord{}, ErrInvalidStudent
	}
	var counselorID *int64
	if staffID, err := s.repo.StaffIDByUserID(ctx, actor.UserID); err == nil && staffID > 0 {
		counselorID = &staffID
	}
	return s.repo.CreateCounselingRecord(ctx, actor.SchoolID, counselorID, in, tanggal.Time())
}

func (s *Service) UpdateCounselingRecord(ctx context.Context, actor Actor, id int64, in UpdateCounselingInput) (CounselingRecord, error) {
	if err := s.authorize(actor); err != nil {
		return CounselingRecord{}, err
	}
	if id <= 0 {
		return CounselingRecord{}, ErrNotFound
	}
	in.Topik = strings.TrimSpace(in.Topik)
	if in.Topik == "" {
		return CounselingRecord{}, ErrInvalidPayload
	}
	tanggal, err := ParseDate(in.Tanggal)
	if err != nil {
		return CounselingRecord{}, err
	}
	if !validCounselingStatus(in.Status) {
		return CounselingRecord{}, ErrInvalidStatus
	}
	return s.repo.UpdateCounselingRecord(ctx, actor.SchoolID, id, in, tanggal.Time())
}

func (s *Service) DeleteCounselingRecord(ctx context.Context, actor Actor, id int64) error {
	if err := s.authorize(actor); err != nil {
		return err
	}
	if id <= 0 {
		return ErrNotFound
	}
	return s.repo.DeleteCounselingRecord(ctx, actor.SchoolID, id)
}

func (s *Service) ListParentSummons(ctx context.Context, actor Actor, filter ListSummonsFilter) ([]ParentSummons, error) {
	if err := s.authorize(actor); err != nil {
		return nil, err
	}
	return s.repo.ListParentSummons(ctx, actor.SchoolID, filter)
}

func (s *Service) CreateParentSummons(ctx context.Context, actor Actor, in CreateParentSummonsInput) (ParentSummons, error) {
	if err := s.authorize(actor); err != nil {
		return ParentSummons{}, err
	}
	if in.StudentID <= 0 {
		return ParentSummons{}, ErrInvalidStudent
	}
	in.NomorSurat = strings.TrimSpace(in.NomorSurat)
	in.Ruangan = strings.TrimSpace(in.Ruangan)
	in.Alasan = strings.TrimSpace(in.Alasan)
	if in.NomorSurat == "" || in.Ruangan == "" || in.Alasan == "" {
		return ParentSummons{}, ErrInvalidPayload
	}
	tanggalSurat, err := ParseDate(in.TanggalSurat)
	if err != nil {
		return ParentSummons{}, err
	}
	jadwal, err := ParseDateTime(in.JadwalPertemuan)
	if err != nil {
		return ParentSummons{}, err
	}
	if in.Status == "" {
		in.Status = "draft"
	}
	if !validSummonsStatus(in.Status) {
		return ParentSummons{}, ErrInvalidStatus
	}
	okStudent, err := s.repo.StudentInSchool(ctx, in.StudentID, actor.SchoolID)
	if err != nil {
		return ParentSummons{}, fmt.Errorf("verify student: %w", err)
	}
	if !okStudent {
		return ParentSummons{}, ErrInvalidStudent
	}
	var createdBy *int64
	if staffID, err := s.repo.StaffIDByUserID(ctx, actor.UserID); err == nil && staffID > 0 {
		createdBy = &staffID
	}
	return s.repo.CreateParentSummons(ctx, actor.SchoolID, createdBy, in, tanggalSurat.Time(), jadwal)
}

func (s *Service) UpdateParentSummons(ctx context.Context, actor Actor, id int64, in UpdateParentSummonsInput) (ParentSummons, error) {
	if err := s.authorize(actor); err != nil {
		return ParentSummons{}, err
	}
	if id <= 0 {
		return ParentSummons{}, ErrNotFound
	}
	in.NomorSurat = strings.TrimSpace(in.NomorSurat)
	in.Ruangan = strings.TrimSpace(in.Ruangan)
	in.Alasan = strings.TrimSpace(in.Alasan)
	if in.NomorSurat == "" || in.Ruangan == "" || in.Alasan == "" {
		return ParentSummons{}, ErrInvalidPayload
	}
	tanggalSurat, err := ParseDate(in.TanggalSurat)
	if err != nil {
		return ParentSummons{}, err
	}
	jadwal, err := ParseDateTime(in.JadwalPertemuan)
	if err != nil {
		return ParentSummons{}, err
	}
	return s.repo.UpdateParentSummons(ctx, actor.SchoolID, id, in, tanggalSurat.Time(), jadwal)
}

func (s *Service) UpdateParentSummonsStatus(ctx context.Context, actor Actor, id int64, status string, catatanHasil *string) (ParentSummons, error) {
	if err := s.authorize(actor); err != nil {
		return ParentSummons{}, err
	}
	if id <= 0 {
		return ParentSummons{}, ErrNotFound
	}
	if !validSummonsStatus(status) {
		return ParentSummons{}, ErrInvalidStatus
	}
	return s.repo.UpdateParentSummonsStatus(ctx, actor.SchoolID, id, status, catatanHasil)
}

func (s *Service) DeleteParentSummons(ctx context.Context, actor Actor, id int64) error {
	if err := s.authorize(actor); err != nil {
		return err
	}
	if id <= 0 {
		return ErrNotFound
	}
	return s.repo.DeleteParentSummons(ctx, actor.SchoolID, id)
}
