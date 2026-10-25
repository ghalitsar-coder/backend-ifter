package subjectattendance

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

// ListSubjects is GET /subjects: every staff role may read the catalog.
func (s *Service) ListSubjects(ctx context.Context, actor Actor, filter SubjectFilter) (SubjectList, error) {
	if err := requireReadActor(actor); err != nil {
		return SubjectList{}, err
	}
	search, err := cleanText(filter.Search, maxNameRunes)
	if err != nil {
		return SubjectList{}, err
	}
	rows, err := s.queries.ListSubjects(ctx, db.ListSubjectsParams{
		Search:   optionalText(search),
		IsActive: boolParam(filter.IsActive),
	})
	if err != nil {
		return SubjectList{}, fmt.Errorf("list subjects: %w", err)
	}
	items := make([]Subject, 0, len(rows))
	for _, row := range rows {
		items = append(items, subjectOf(row))
	}
	return SubjectList{Data: items}, nil
}

// GetSubject is GET /subjects/:id.
func (s *Service) GetSubject(ctx context.Context, actor Actor, idValue string) (Subject, error) {
	if err := requireReadActor(actor); err != nil {
		return Subject{}, err
	}
	id, err := parseUUID(idValue)
	if err != nil {
		return Subject{}, ErrInvalidInput
	}
	row, err := s.queries.GetSubject(ctx, id)
	if err != nil {
		if database.IsNotFound(err) {
			return Subject{}, ErrNotFound
		}
		return Subject{}, fmt.Errorf("get subject: %w", err)
	}
	return subjectOf(row), nil
}

// CreateSubject is POST /subjects (admin + staff_tu), audited as subject.create.
func (s *Service) CreateSubject(ctx context.Context, actor Actor, input SubjectInput, requestID string) (Subject, error) {
	if err := requireMasterActor(actor); err != nil {
		return Subject{}, err
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return Subject{}, ErrUnauthorized
	}
	code, name, description, err := validateSubjectInput(input)
	if err != nil {
		return Subject{}, err
	}
	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	var created db.Subject
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		row, err := q.CreateSubject(ctx, db.CreateSubjectParams{
			Code:        code,
			Name:        name,
			Description: description,
			IsActive:    isActive,
		})
		if err != nil {
			return err
		}
		created = row
		return s.audit(ctx, q, actorID, "subject.create", "subject", row.ID, nil, subjectOf(row), requestID)
	})
	if err != nil {
		return Subject{}, mapWriteError(err)
	}
	return subjectOf(created), nil
}

// UpdateSubject is PUT /subjects/:id, audited as subject.update with before/after.
func (s *Service) UpdateSubject(ctx context.Context, actor Actor, idValue string, input SubjectInput, requestID string) (Subject, error) {
	if err := requireMasterActor(actor); err != nil {
		return Subject{}, err
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return Subject{}, ErrUnauthorized
	}
	id, err := parseUUID(idValue)
	if err != nil {
		return Subject{}, ErrInvalidInput
	}
	code, name, description, err := validateSubjectInput(input)
	if err != nil {
		return Subject{}, err
	}
	before, err := s.queries.GetSubject(ctx, id)
	if err != nil {
		if database.IsNotFound(err) {
			return Subject{}, ErrNotFound
		}
		return Subject{}, fmt.Errorf("get subject: %w", err)
	}
	isActive := before.IsActive
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	var updated db.Subject
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		row, err := q.UpdateSubject(ctx, db.UpdateSubjectParams{
			Code:        code,
			Name:        name,
			Description: description,
			IsActive:    isActive,
			ID:          id,
		})
		if err != nil {
			return err
		}
		updated = row
		return s.audit(ctx, q, actorID, "subject.update", "subject", id, subjectOf(before), subjectOf(row), requestID)
	})
	if err != nil {
		return Subject{}, mapWriteError(err)
	}
	return subjectOf(updated), nil
}

func validateSubjectInput(input SubjectInput) (string, string, pgtype.Text, error) {
	code, err := cleanText(input.Code, maxCodeRunes)
	if err != nil || code == "" || strings.ContainsAny(code, " \t\r\n") {
		return "", "", pgtype.Text{}, ErrInvalidInput
	}
	name, err := cleanText(input.Name, maxNameRunes)
	if err != nil || name == "" {
		return "", "", pgtype.Text{}, ErrInvalidInput
	}
	description, err := cleanText(input.Description, maxNotesRunes)
	if err != nil {
		return "", "", pgtype.Text{}, err
	}
	return code, name, optionalText(description), nil
}

func subjectOf(row db.Subject) Subject {
	return Subject{
		ID:          uuidString(row.ID),
		Code:        row.Code,
		Name:        row.Name,
		Description: textPtr(row.Description),
		IsActive:    row.IsActive,
		CreatedAt:   timestamp(row.CreatedAt),
		UpdatedAt:   timestamp(row.UpdatedAt),
	}
}

func boolParam(value *bool) pgtype.Bool {
	if value == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *value, Valid: true}
}
