package recap

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

func (s *Service) GenerateStudentRecaps(ctx context.Context, schoolID int64, month, year int) ([]StudentRecap, error) {
	monthValue, yearValue, err := validatePeriod(schoolID, month, year)
	if err != nil {
		return nil, err
	}
	if err := s.repo.GenerateStudentRecaps(ctx, schoolID, monthValue, yearValue); err != nil {
		return nil, fmt.Errorf("generate student recaps: %w", err)
	}
	return s.ListStudentRecaps(ctx, schoolID, month, year)
}

func (s *Service) ListStudentRecaps(ctx context.Context, schoolID int64, month, year int) ([]StudentRecap, error) {
	monthValue, yearValue, err := validatePeriod(schoolID, month, year)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.ListStudentRecaps(ctx, schoolID, monthValue, yearValue)
	if err != nil {
		return nil, fmt.Errorf("list student recaps: %w", err)
	}
	return items, nil
}

func (s *Service) VerifyStudentRecap(ctx context.Context, id, schoolID, userID int64) (Verification, error) {
	if id <= 0 {
		return Verification{}, ErrInvalidID
	}
	if schoolID <= 0 || userID <= 0 {
		return Verification{}, ErrForbidden
	}
	verified, err := s.repo.VerifyStudentRecap(ctx, id, schoolID, userID)
	if err == nil {
		return verified, nil
	}
	if !database.IsNotFound(err) {
		return Verification{}, fmt.Errorf("verify student recap: %w", err)
	}

	status, statusErr := s.repo.StudentRecapStatus(ctx, id, schoolID)
	if database.IsNotFound(statusErr) {
		return Verification{}, ErrNotFound
	}
	if statusErr != nil {
		return Verification{}, fmt.Errorf("check student recap status: %w", statusErr)
	}
	if status == "dikunci" {
		return Verification{}, ErrLocked
	}
	return Verification{}, fmt.Errorf("verify student recap: unexpected status %q", status)
}

func (s *Service) GenerateStaffRecaps(ctx context.Context, schoolID int64, month, year int) ([]StaffRecap, error) {
	monthValue, yearValue, err := validatePeriod(schoolID, month, year)
	if err != nil {
		return nil, err
	}
	if err := s.repo.GenerateStaffRecaps(ctx, schoolID, monthValue, yearValue); err != nil {
		return nil, fmt.Errorf("generate staff recaps: %w", err)
	}
	return s.ListStaffRecaps(ctx, schoolID, month, year)
}

func (s *Service) ListStaffRecaps(ctx context.Context, schoolID int64, month, year int) ([]StaffRecap, error) {
	monthValue, yearValue, err := validatePeriod(schoolID, month, year)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.ListStaffRecaps(ctx, schoolID, monthValue, yearValue)
	if err != nil {
		return nil, fmt.Errorf("list staff recaps: %w", err)
	}
	return items, nil
}

func (s *Service) VerifyStaffRecap(ctx context.Context, id, schoolID, userID int64) (Verification, error) {
	if id <= 0 {
		return Verification{}, ErrInvalidID
	}
	if schoolID <= 0 || userID <= 0 {
		return Verification{}, ErrForbidden
	}
	verified, err := s.repo.VerifyStaffRecap(ctx, id, schoolID, userID)
	if err == nil {
		return verified, nil
	}
	if !database.IsNotFound(err) {
		return Verification{}, fmt.Errorf("verify staff recap: %w", err)
	}

	status, statusErr := s.repo.StaffRecapStatus(ctx, id, schoolID)
	if database.IsNotFound(statusErr) {
		return Verification{}, ErrNotFound
	}
	if statusErr != nil {
		return Verification{}, fmt.Errorf("check staff recap status: %w", statusErr)
	}
	if status == "dikunci" {
		return Verification{}, ErrLocked
	}
	return Verification{}, fmt.Errorf("verify staff recap: unexpected status %q", status)
}

func validatePeriod(schoolID int64, month, year int) (int16, int16, error) {
	if schoolID <= 0 {
		return 0, 0, ErrForbidden
	}
	if month < 1 || month > 12 || year < 1 || year > 9999 {
		return 0, 0, ErrInvalidPeriod
	}
	return int16(month), int16(year), nil
}
