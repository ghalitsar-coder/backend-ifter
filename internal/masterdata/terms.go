package masterdata

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

func (s *Service) ListTerms(ctx context.Context, academicYearID string) ([]Term, error) {
	yearID, err := parseUUID(academicYearID)
	if err != nil {
		return nil, ErrInvalidInput
	}
	rows, err := s.queries.ListTerms(ctx, yearID)
	if err != nil {
		return nil, fmt.Errorf("list terms: %w", err)
	}
	terms := make([]Term, 0, len(rows))
	for _, row := range rows {
		terms = append(terms, toTerm(row))
	}
	return terms, nil
}

func (s *Service) GetTerm(ctx context.Context, id string) (Term, error) {
	termID, err := parseUUID(id)
	if err != nil {
		return Term{}, ErrInvalidInput
	}
	row, err := s.queries.GetTerm(ctx, termID)
	if err != nil {
		if database.IsNotFound(err) {
			return Term{}, ErrNotFound
		}
		return Term{}, fmt.Errorf("get term: %w", err)
	}
	return toTerm(row), nil
}

func (s *Service) CreateTerm(ctx context.Context, actorID, academicYearID string, input TermInput, requestID string) (Term, error) {
	actorUUID, err := parseUUID(actorID)
	if err != nil {
		return Term{}, ErrUnauthorized
	}
	yearID, err := parseUUID(academicYearID)
	if err != nil {
		return Term{}, ErrInvalidInput
	}
	termNumber, name, start, end, err := validateTermInput(input)
	if err != nil {
		return Term{}, err
	}
	year, err := s.queries.GetAcademicYear(ctx, yearID)
	if err != nil {
		if database.IsNotFound(err) {
			return Term{}, ErrNotFound
		}
		return Term{}, fmt.Errorf("get academic year for term: %w", err)
	}
	if start.Time.Before(year.StartDate.Time) || end.Time.After(year.EndDate.Time) {
		return Term{}, ErrInvalidInput
	}

	var result Term
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		row, err := q.CreateTerm(ctx, db.CreateTermParams{
			AcademicYearID: yearID,
			TermNumber:     termNumber,
			Name:           name,
			StartDate:      start,
			EndDate:        end,
		})
		if err != nil {
			return mapWriteError(fmt.Errorf("create term: %w", err))
		}
		result = toTerm(row)
		if err := s.auditData(ctx, q, actorUUID, "term.create", "term", row.ID, nil, result, requestID); err != nil {
			return fmt.Errorf("write term audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return Term{}, mapWriteError(err)
	}
	return result, nil
}

func (s *Service) UpdateTerm(ctx context.Context, actorID, id string, input TermInput, requestID string) (Term, error) {
	actorUUID, err := parseUUID(actorID)
	if err != nil {
		return Term{}, ErrUnauthorized
	}
	termID, err := parseUUID(id)
	if err != nil {
		return Term{}, ErrInvalidInput
	}
	termNumber, name, start, end, err := validateTermInput(input)
	if err != nil {
		return Term{}, err
	}

	var result Term
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		existing, err := q.GetTermForUpdate(ctx, termID)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("get term for update: %w", err)
		}
		year, err := q.GetAcademicYear(ctx, existing.AcademicYearID)
		if err != nil {
			return fmt.Errorf("get academic year for term: %w", err)
		}
		if start.Time.Before(year.StartDate.Time) || end.Time.After(year.EndDate.Time) {
			return ErrInvalidInput
		}
		row, err := q.UpdateTerm(ctx, db.UpdateTermParams{
			TermNumber: termNumber,
			Name:       name,
			StartDate:  start,
			EndDate:    end,
			ID:         termID,
		})
		if err != nil {
			return mapWriteError(fmt.Errorf("update term: %w", err))
		}
		result = toTerm(row)
		if err := s.auditData(ctx, q, actorUUID, "term.update", "term", row.ID, toTerm(existing), result, requestID); err != nil {
			return fmt.Errorf("write term audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return Term{}, mapWriteError(err)
	}
	return result, nil
}

func validateTermInput(input TermInput) (int16, string, pgtype.Date, pgtype.Date, error) {
	if input.TermNumber < 1 || input.TermNumber > 4 {
		return 0, "", pgtype.Date{}, pgtype.Date{}, ErrInvalidInput
	}
	name, err := requiredText(input.Name, 30)
	if err != nil {
		return 0, "", pgtype.Date{}, pgtype.Date{}, err
	}
	start, err := parseDate(input.StartDate)
	if err != nil {
		return 0, "", pgtype.Date{}, pgtype.Date{}, ErrInvalidInput
	}
	end, err := parseDate(input.EndDate)
	if err != nil || !end.Time.After(start.Time) {
		return 0, "", pgtype.Date{}, pgtype.Date{}, ErrInvalidInput
	}
	return input.TermNumber, name, start, end, nil
}

func toTerm(row db.Term) Term {
	return Term{
		ID:             uuidString(row.ID),
		AcademicYearID: uuidString(row.AcademicYearID),
		TermNumber:     row.TermNumber,
		Name:           row.Name,
		StartDate:      dateString(row.StartDate),
		EndDate:        dateString(row.EndDate),
	}
}
