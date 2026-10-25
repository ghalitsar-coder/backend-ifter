package dispensations

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

// Create records a new dispensation request on behalf of a student
// (POST /dispensations; staff roles). The class_id snapshot is taken from the
// student's current enrollment at create time; a student without a current
// enrollment is still accepted (class_id stays null). A plain teacher may only
// create for students of a class they currently homeroom (the same server-side
// scoping as List/Get/Verify); admin, staff_tu and duty_teacher are school-wide.
func (s *Service) Create(ctx context.Context, actor Actor, in CreateInput) (Item, error) {
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return Item{}, ErrUnauthorized
	}
	studentID, err := parseUUID(in.StudentID)
	if err != nil {
		return Item{}, ErrInvalidInput
	}
	scope, err := s.resolveReadScope(ctx, actor)
	if err != nil {
		return Item{}, err
	}
	if !scope.all {
		student, err := s.queries.GetStudent(ctx, studentID)
		if err != nil {
			if database.IsNotFound(err) {
				return Item{}, ErrNotFound
			}
			return Item{}, fmt.Errorf("get student: %w", err)
		}
		if err := checkClassAccess(scope, student.CurrentClassID); err != nil {
			return Item{}, err
		}
	}
	return s.create(ctx, actorID, studentID, in)
}

// CreateMine records a dispensation for the actor's own student record
// (POST /my/dispensations); the student is always resolved from the auth
// subject, never from a client-supplied id.
func (s *Service) CreateMine(ctx context.Context, actor Actor, in CreateInput) (Item, error) {
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return Item{}, ErrUnauthorized
	}
	studentID, err := s.studentIDForActor(ctx, actor)
	if err != nil {
		return Item{}, err
	}
	in.StudentID = ""
	return s.create(ctx, actorID, studentID, in)
}

func (s *Service) create(ctx context.Context, actorID, studentID pgtype.UUID, in CreateInput) (Item, error) {
	if !validType(in.Type) {
		return Item{}, ErrInvalidInput
	}
	reason, err := cleanText(in.Reason, 1000)
	if err != nil || reason == "" {
		return Item{}, ErrInvalidInput
	}
	startDate, err := parseDate(in.StartDate)
	if err != nil {
		return Item{}, ErrInvalidInput
	}
	endDate := pgtype.Date{}
	if in.EndDate != "" {
		endDate, err = parseDate(in.EndDate)
		if err != nil {
			return Item{}, ErrInvalidInput
		}
		if endDate.Time.Before(startDate.Time) {
			return Item{}, ErrInvalidInput
		}
	}

	student, err := s.queries.GetStudent(ctx, studentID)
	if err != nil {
		if database.IsNotFound(err) {
			return Item{}, ErrNotFound
		}
		return Item{}, fmt.Errorf("get student: %w", err)
	}
	if student.Status != "active" {
		return Item{}, ErrInvalidInput
	}

	params := db.CreateDispensationParams{
		StudentID:   studentID,
		ClassID:     student.CurrentClassID,
		Type:        in.Type,
		Reason:      reason,
		StartDate:   startDate,
		EndDate:     endDate,
		SubmittedBy: actorID,
	}
	if doc := in.Document; doc != nil && len(doc.Data) > 0 {
		filename, storedKey, err := s.storeDocument(doc)
		if err != nil {
			return Item{}, err
		}
		params.DocumentFileName = pgtype.Text{String: filename, Valid: true}
		params.DocumentStoredKey = pgtype.Text{String: storedKey, Valid: true}
	}

	created, err := s.queries.CreateDispensation(ctx, params)
	if err != nil {
		if params.DocumentStoredKey.Valid {
			_ = s.removeFile(params.DocumentStoredKey.String)
		}
		return Item{}, mapWriteError(fmt.Errorf("create dispensation: %w", err))
	}
	return s.fetchItem(ctx, created.ID)
}

// List returns dispensations visible to the actor (teachers are scoped to
// their own homeroom classes).
func (s *Service) List(ctx context.Context, actor Actor, filter ListFilter) (ListResult, error) {
	scope, err := s.resolveReadScope(ctx, actor)
	if err != nil {
		return ListResult{}, err
	}
	params, err := s.buildListParams(filter)
	if err != nil {
		return ListResult{}, err
	}
	if !scope.all {
		if len(scope.classIDs) == 0 {
			limit, offset := normalizePage(filter.Limit, filter.Offset)
			return ListResult{Data: []Item{}, Total: 0, Limit: limit, Offset: offset}, nil
		}
		params.ScopeClassIds = scope.classIDs
	}
	return s.queryList(ctx, params)
}

// ListMine lists the actor's own dispensations (GET /my/dispensations).
func (s *Service) ListMine(ctx context.Context, actor Actor, filter ListFilter) (ListResult, error) {
	studentID, err := s.studentIDForActor(ctx, actor)
	if err != nil {
		return ListResult{}, err
	}
	params, err := s.buildListParams(filter)
	if err != nil {
		return ListResult{}, err
	}
	params.StudentID = studentID
	return s.queryList(ctx, params)
}

func (s *Service) buildListParams(filter ListFilter) (db.ListDispensationsParams, error) {
	params := db.ListDispensationsParams{}
	params.Status = optionalText(filter.Status)
	params.Type = optionalText(filter.Type)
	if filter.Status != "" && !validStatus(filter.Status) {
		return params, ErrInvalidInput
	}
	if filter.Type != "" && !validType(filter.Type) {
		return params, ErrInvalidInput
	}
	if filter.StudentID != "" {
		id, err := parseUUID(filter.StudentID)
		if err != nil {
			return params, ErrInvalidInput
		}
		params.StudentID = id
	}
	if filter.ClassID != "" {
		id, err := parseUUID(filter.ClassID)
		if err != nil {
			return params, ErrInvalidInput
		}
		params.ClassID = id
	}
	if filter.From != "" {
		date, err := parseDate(filter.From)
		if err != nil {
			return params, ErrInvalidInput
		}
		params.FromDate = date
	}
	if filter.To != "" {
		date, err := parseDate(filter.To)
		if err != nil {
			return params, ErrInvalidInput
		}
		params.ToDate = date
	}
	if params.FromDate.Valid && params.ToDate.Valid && params.ToDate.Time.Before(params.FromDate.Time) {
		return params, ErrInvalidInput
	}
	params.PageLimit, params.PageOffset = normalizePage(filter.Limit, filter.Offset)
	return params, nil
}

func (s *Service) queryList(ctx context.Context, params db.ListDispensationsParams) (ListResult, error) {
	total, err := s.queries.CountDispensations(ctx, db.CountDispensationsParams{
		Status:        params.Status,
		Type:          params.Type,
		StudentID:     params.StudentID,
		ClassID:       params.ClassID,
		ScopeClassIds: params.ScopeClassIds,
		FromDate:      params.FromDate,
		ToDate:        params.ToDate,
	})
	if err != nil {
		return ListResult{}, fmt.Errorf("count dispensations: %w", err)
	}
	rows, err := s.queries.ListDispensations(ctx, params)
	if err != nil {
		return ListResult{}, fmt.Errorf("list dispensations: %w", err)
	}
	data := make([]Item, 0, len(rows))
	for _, row := range rows {
		data = append(data, itemFromListRow(row))
	}
	return ListResult{
		Data:   data,
		Total:  total,
		Limit:  params.PageLimit,
		Offset: params.PageOffset,
	}, nil
}

// Get returns one dispensation with the same read scoping as List.
func (s *Service) Get(ctx context.Context, actor Actor, id string) (Item, error) {
	scope, err := s.resolveReadScope(ctx, actor)
	if err != nil {
		return Item{}, err
	}
	item, err := s.getItem(ctx, id)
	if err != nil {
		return Item{}, err
	}
	if err := checkClassAccess(scope, item.classID); err != nil {
		return Item{}, err
	}
	return item.Item, nil
}

type dispensationDetail struct {
	Item
	classID pgtype.UUID
}

func (s *Service) getItem(ctx context.Context, id string) (dispensationDetail, error) {
	uuid, err := parseUUID(id)
	if err != nil {
		return dispensationDetail{}, ErrInvalidInput
	}
	row, err := s.queries.GetDispensation(ctx, uuid)
	if err != nil {
		if database.IsNotFound(err) {
			return dispensationDetail{}, ErrNotFound
		}
		return dispensationDetail{}, fmt.Errorf("get dispensation: %w", err)
	}
	return dispensationDetail{Item: itemFromGetRow(row), classID: row.ClassID}, nil
}

func (s *Service) fetchItem(ctx context.Context, id pgtype.UUID) (Item, error) {
	row, err := s.queries.GetDispensation(ctx, id)
	if err != nil {
		if database.IsNotFound(err) {
			return Item{}, ErrNotFound
		}
		return Item{}, fmt.Errorf("fetch created dispensation: %w", err)
	}
	return itemFromGetRow(row), nil
}

// storeDocument validates and persists an upload into the private imports
// storage dir under a random opaque key. It never trusts the client filename
// for the storage path.
func (s *Service) storeDocument(doc *DocumentInput) (string, string, error) {
	if int64(len(doc.Data)) > s.maxFileSize {
		return "", "", ErrFileTooLarge
	}
	extension, err := validateDocument(doc.Filename, doc.ContentType, doc.Data)
	if err != nil {
		return "", "", err
	}
	filename, err := cleanText(filepath.Base(doc.Filename), 255)
	if err != nil || filename == "." || filename == "" {
		return "", "", ErrInvalidInput
	}
	if err := s.ensureDir(); err != nil {
		return "", "", err
	}
	storedKey := uuid.NewString() + extension
	if err := s.writeFile(storedKey, doc.Data); err != nil {
		return "", "", fmt.Errorf("store dispensation document: %w", err)
	}
	return filename, storedKey, nil
}
