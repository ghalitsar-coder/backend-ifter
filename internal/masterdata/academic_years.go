package masterdata

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

func (s *Service) ListAcademicYears(ctx context.Context) ([]AcademicYear, error) {
	rows, err := s.queries.ListAcademicYears(ctx)
	if err != nil {
		return nil, fmt.Errorf("list academic years: %w", err)
	}
	years := make([]AcademicYear, 0, len(rows))
	for _, row := range rows {
		years = append(years, toAcademicYear(row))
	}
	return years, nil
}

func (s *Service) GetAcademicYear(ctx context.Context, id string) (AcademicYear, error) {
	yearID, err := parseUUID(id)
	if err != nil {
		return AcademicYear{}, ErrInvalidInput
	}
	row, err := s.queries.GetAcademicYear(ctx, yearID)
	if err != nil {
		if database.IsNotFound(err) {
			return AcademicYear{}, ErrNotFound
		}
		return AcademicYear{}, fmt.Errorf("get academic year: %w", err)
	}
	return toAcademicYear(row), nil
}

func (s *Service) CreateAcademicYear(ctx context.Context, actorID string, input AcademicYearInput, requestID string) (AcademicYear, error) {
	actorUUID, err := parseUUID(actorID)
	if err != nil {
		return AcademicYear{}, ErrUnauthorized
	}
	params, err := validateAcademicYearInput(input)
	if err != nil {
		return AcademicYear{}, err
	}

	var result AcademicYear
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		if err := q.LockAcademicYearActivation(ctx); err != nil {
			return fmt.Errorf("lock academic year activation: %w", err)
		}
		if params.IsActive {
			if err := q.DeactivateAllAcademicYears(ctx); err != nil {
				return fmt.Errorf("deactivate academic years: %w", err)
			}
		}
		row, err := q.CreateAcademicYear(ctx, params)
		if err != nil {
			return mapWriteError(err)
		}
		result = toAcademicYear(row)
		if err := s.auditData(ctx, q, actorUUID, "academic_year.create", "academic_year", row.ID, nil, result, requestID); err != nil {
			return fmt.Errorf("write academic year audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return AcademicYear{}, mapWriteError(err)
	}
	return result, nil
}

func (s *Service) UpdateAcademicYear(ctx context.Context, actorID, id string, input AcademicYearInput, requestID string) (AcademicYear, error) {
	actorUUID, err := parseUUID(actorID)
	if err != nil {
		return AcademicYear{}, ErrUnauthorized
	}
	yearID, err := parseUUID(id)
	if err != nil {
		return AcademicYear{}, ErrInvalidInput
	}
	params, err := validateAcademicYearInput(input)
	if err != nil {
		return AcademicYear{}, err
	}

	var result AcademicYear
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		if err := q.LockAcademicYearActivation(ctx); err != nil {
			return fmt.Errorf("lock academic year activation: %w", err)
		}
		existing, err := q.GetAcademicYearForUpdate(ctx, yearID)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("get academic year for update: %w", err)
		}
		before := toAcademicYear(existing)

		outside, err := q.CountDatedRecordsOutsideAcademicYear(ctx, db.CountDatedRecordsOutsideAcademicYearParams{
			ID:        yearID,
			StartDate: params.StartDate,
			EndDate:   params.EndDate,
		})
		if err != nil {
			return fmt.Errorf("check academic year dated records: %w", err)
		}
		if outside > 0 && (!params.StartDate.Time.Equal(existing.StartDate.Time) || !params.EndDate.Time.Equal(existing.EndDate.Time)) {
			return ErrConflict
		}
		if params.IsActive {
			if err := q.DeactivateOtherAcademicYears(ctx, yearID); err != nil {
				return fmt.Errorf("deactivate other academic years: %w", err)
			}
		}
		row, err := q.UpdateAcademicYear(ctx, db.UpdateAcademicYearParams{
			Name:      params.Name,
			StartDate: params.StartDate,
			EndDate:   params.EndDate,
			IsActive:  params.IsActive,
			ID:        yearID,
		})
		if err != nil {
			return mapWriteError(err)
		}
		result = toAcademicYear(row)
		if err := s.auditData(ctx, q, actorUUID, "academic_year.update", "academic_year", row.ID, before, result, requestID); err != nil {
			return fmt.Errorf("write academic year audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return AcademicYear{}, mapWriteError(err)
	}
	return result, nil
}

func validateAcademicYearInput(input AcademicYearInput) (db.CreateAcademicYearParams, error) {
	name, err := requiredText(input.Name, 30)
	if err != nil {
		return db.CreateAcademicYearParams{}, err
	}
	start, err := parseDate(input.StartDate)
	if err != nil {
		return db.CreateAcademicYearParams{}, ErrInvalidInput
	}
	end, err := parseDate(input.EndDate)
	if err != nil || !end.Time.After(start.Time) {
		return db.CreateAcademicYearParams{}, ErrInvalidInput
	}
	return db.CreateAcademicYearParams{
		Name:      name,
		StartDate: start,
		EndDate:   end,
		IsActive:  input.IsActive,
	}, nil
}

func toAcademicYear(row db.AcademicYear) AcademicYear {
	return AcademicYear{
		ID:        uuidString(row.ID),
		Name:      row.Name,
		StartDate: dateString(row.StartDate),
		EndDate:   dateString(row.EndDate),
		IsActive:  row.IsActive,
		CreatedAt: timestamp(row.CreatedAt),
		UpdatedAt: timestamp(row.UpdatedAt),
	}
}
