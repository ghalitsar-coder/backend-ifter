package attendance

import (
	"context"
	"fmt"

	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// CreateSession is idempotent: the unique index on (class, tanggal, mapel)
// returns the existing session instead of failing on a duplicate submit.
func (s *Service) CreateSession(ctx context.Context, actor Actor, in CreateSessionInput) (Session, error) {
	if in.ClassID <= 0 {
		return Session{}, ErrInvalidClass
	}
	if in.ClassSubjectID != nil && *in.ClassSubjectID <= 0 {
		return Session{}, ErrInvalidSubject
	}
	tanggal, err := ParseDate(in.Tanggal)
	if err != nil {
		return Session{}, err
	}

	staffID, err := s.staffID(ctx, actor)
	if err != nil {
		return Session{}, err
	}
	if !actor.Manage {
		allowed, err := s.repo.CanRecord(ctx, staffID, in.ClassID, in.ClassSubjectID)
		if err != nil {
			return Session{}, fmt.Errorf("check class ownership: %w", err)
		}
		if !allowed {
			return Session{}, ErrForbidden
		}
	}

	created, err := s.repo.CreateSession(ctx, in.ClassID, staffID, in.ClassSubjectID, tanggal.Time())
	if err != nil {
		return Session{}, fmt.Errorf("create attendance session: %w", err)
	}
	return created, nil
}

func (s *Service) GetSession(ctx context.Context, id int64) (SessionDetail, error) {
	session, err := s.repo.GetSession(ctx, id)
	if err != nil {
		if database.IsNotFound(err) {
			return SessionDetail{}, ErrNotFound
		}
		return SessionDetail{}, fmt.Errorf("get attendance session: %w", err)
	}

	rows, err := s.repo.ListAttendances(ctx, session.ID, session.ClassID)
	if err != nil {
		return SessionDetail{}, fmt.Errorf("list attendances: %w", err)
	}
	return SessionDetail{Session: session, Attendances: rows}, nil
}

func (s *Service) Record(ctx context.Context, actor Actor, in RecordInput) (SessionDetail, error) {
	if err := validateEntries(in.Attendances); err != nil {
		return SessionDetail{}, err
	}

	session, err := s.repo.GetSession(ctx, in.SessionID)
	if err != nil {
		if database.IsNotFound(err) {
			return SessionDetail{}, ErrNotFound
		}
		return SessionDetail{}, fmt.Errorf("get attendance session: %w", err)
	}
	if err := s.authorize(ctx, actor, session); err != nil {
		return SessionDetail{}, err
	}
	if session.IsLocked {
		return SessionDetail{}, ErrLocked
	}

	ids := make([]int64, 0, len(in.Attendances))
	for _, entry := range in.Attendances {
		ids = append(ids, entry.StudentID)
	}
	enrolled, err := s.repo.CountEnrolledStudents(ctx, session.ClassID, ids)
	if err != nil {
		return SessionDetail{}, fmt.Errorf("count enrolled students: %w", err)
	}
	if enrolled != int64(len(ids)) {
		return SessionDetail{}, ErrStudentNotEnrolled
	}

	if err := s.repo.RecordAttendances(ctx, session.ID, in.Attendances); err != nil {
		return SessionDetail{}, err
	}

	rows, err := s.repo.ListAttendances(ctx, session.ID, session.ClassID)
	if err != nil {
		return SessionDetail{}, fmt.Errorf("list attendances: %w", err)
	}
	return SessionDetail{Session: session, Attendances: rows}, nil
}

func (s *Service) Lock(ctx context.Context, actor Actor, id int64) (Session, error) {
	session, err := s.repo.GetSession(ctx, id)
	if err != nil {
		if database.IsNotFound(err) {
			return Session{}, ErrNotFound
		}
		return Session{}, fmt.Errorf("get attendance session: %w", err)
	}
	if err := s.authorize(ctx, actor, session); err != nil {
		return Session{}, err
	}

	locked, err := s.repo.LockSession(ctx, session.ID)
	if err != nil {
		if database.IsNotFound(err) {
			return Session{}, ErrNotFound
		}
		return Session{}, fmt.Errorf("lock attendance session: %w", err)
	}
	return locked, nil
}

func (s *Service) staffID(ctx context.Context, actor Actor) (int64, error) {
	staffID, err := s.repo.StaffIDByUserID(ctx, actor.UserID)
	if err != nil {
		if database.IsNotFound(err) {
			return 0, ErrNotStaff
		}
		return 0, fmt.Errorf("resolve staff: %w", err)
	}
	return staffID, nil
}

func (s *Service) authorize(ctx context.Context, actor Actor, session Session) error {
	if actor.Manage {
		return nil
	}
	staffID, err := s.staffID(ctx, actor)
	if err != nil {
		return err
	}
	allowed, err := s.repo.CanRecord(ctx, staffID, session.ClassID, session.ClassSubjectID)
	if err != nil {
		return fmt.Errorf("check class ownership: %w", err)
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}

func validateEntries(entries []AttendanceInput) error {
	if len(entries) == 0 {
		return ErrEmptyAttendances
	}
	seen := make(map[int64]struct{}, len(entries))
	for _, entry := range entries {
		if entry.StudentID <= 0 {
			return ErrInvalidStudent
		}
		if !validStatus(entry.Status) {
			return ErrInvalidStatus
		}
		if _, duplicate := seen[entry.StudentID]; duplicate {
			return ErrDuplicateStudent
		}
		seen[entry.StudentID] = struct{}{}
	}
	return nil
}
