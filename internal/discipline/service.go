package discipline

import (
	"context"
	"fmt"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) ListViolationTypes(ctx context.Context, actor Actor) ([]ViolationType, error) {
	if actor.SchoolID <= 0 {
		return nil, ErrForbidden
	}
	return s.repo.ListViolationTypes(ctx, actor.SchoolID)
}

func (s *Service) RecordViolation(ctx context.Context, actor Actor, in RecordViolationInput) (StudentViolation, error) {
	if actor.SchoolID <= 0 {
		return StudentViolation{}, ErrForbidden
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
	if actor.SchoolID <= 0 {
		return nil, ErrForbidden
	}
	return s.repo.ListViolations(ctx, actor.SchoolID, filter)
}

func (s *Service) GetStudentSummary(ctx context.Context, actor Actor, studentID int64) (StudentSummary, error) {
	if actor.SchoolID <= 0 {
		return StudentSummary{}, ErrForbidden
	}
	if studentID <= 0 {
		return StudentSummary{}, ErrInvalidStudent
	}
	return s.repo.GetStudentSummary(ctx, actor.SchoolID, studentID)
}

func (s *Service) UpdateFollowup(ctx context.Context, actor Actor, id int64, followup string) (StudentViolation, error) {
	if actor.SchoolID <= 0 {
		return StudentViolation{}, ErrForbidden
	}
	if id <= 0 {
		return StudentViolation{}, ErrNotFound
	}
	return s.repo.UpdateFollowup(ctx, actor.SchoolID, id, followup)
}

func (s *Service) CreateInspection(ctx context.Context, actor Actor, in CreateInspectionInput) (DisciplineInspection, error) {
	if actor.SchoolID <= 0 {
		return DisciplineInspection{}, ErrForbidden
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

	// Prepare default points map for violations
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
	if actor.SchoolID <= 0 {
		return nil, ErrForbidden
	}
	return s.repo.ListInspections(ctx, actor.SchoolID, filter)
}

func (s *Service) ListClassStudents(ctx context.Context, actor Actor, classID int64) ([]ClassStudent, error) {
	if actor.SchoolID <= 0 {
		return nil, ErrForbidden
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
