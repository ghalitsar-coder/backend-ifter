package masterdata

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

func (s *Service) ListTeachers(ctx context.Context, filter TeacherFilter, page Page) ([]Teacher, int64, error) {
	page = normalizePage(page)
	params := db.ListTeachersParams{
		Search:     pgtype.Text{},
		Status:     pgtype.Text{},
		PageOffset: page.Offset,
		PageLimit:  page.Limit,
	}
	if filter.Status != "" {
		if !validStatus(filter.Status, "active", "inactive") {
			return nil, 0, ErrInvalidInput
		}
		params.Status = optionalText(filter.Status)
	}
	search, err := cleanText(filter.Search, 150)
	if err != nil {
		return nil, 0, err
	}
	params.Search = optionalText(search)

	rows, err := s.queries.ListTeachers(ctx, params)
	if err != nil {
		return nil, 0, fmt.Errorf("list teachers: %w", err)
	}
	total, err := s.queries.CountTeachers(ctx, db.CountTeachersParams{
		Status: params.Status,
		Search: params.Search,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count teachers: %w", err)
	}
	teachers := make([]Teacher, 0, len(rows))
	if hasAnyEmploymentType(rows) {
		// One lookup for the whole page: the employment_types table is master
		// data (a handful of rows), not a per-teacher query (employment-type
		// package, docs/API_ADDENDUM.md §6.5).
		names := s.employmentTypeNames(ctx)
		for _, row := range rows {
			teachers = append(teachers, withEmploymentName(toTeacher(row), names))
		}
		return teachers, total, nil
	}
	for _, row := range rows {
		teachers = append(teachers, toTeacher(row))
	}
	return teachers, total, nil
}

func (s *Service) GetTeacher(ctx context.Context, id string) (Teacher, error) {
	teacherID, err := parseUUID(id)
	if err != nil {
		return Teacher{}, ErrInvalidInput
	}
	row, err := s.queries.GetTeacher(ctx, teacherID)
	if err != nil {
		if database.IsNotFound(err) {
			return Teacher{}, ErrNotFound
		}
		return Teacher{}, fmt.Errorf("get teacher: %w", err)
	}
	name, err := employmentTypeNameByID(ctx, s.queries, row.EmploymentTypeID)
	if err != nil {
		return Teacher{}, err
	}
	result := toTeacher(row)
	result.EmploymentTypeName = name
	return result, nil
}

func (s *Service) CreateTeacher(ctx context.Context, actorID string, input TeacherInput, requestID string) (Teacher, error) {
	actorUUID, err := parseUUID(actorID)
	if err != nil {
		return Teacher{}, ErrUnauthorized
	}
	params, userID, err := validateTeacherInput(input)
	if err != nil {
		return Teacher{}, err
	}

	var result Teacher
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		if userID.Valid {
			if _, err := q.GetUserByID(ctx, userID); err != nil {
				if database.IsNotFound(err) {
					return ErrNotFound
				}
				return fmt.Errorf("find teacher user: %w", err)
			}
		}
		row, err := q.CreateTeacher(ctx, params)
		if err != nil {
			return mapWriteError(err)
		}
		result = toTeacher(row)
		name, err := employmentTypeNameByID(ctx, q, row.EmploymentTypeID)
		if err != nil {
			return err
		}
		result.EmploymentTypeName = name
		if err := s.auditData(ctx, q, actorUUID, "teacher.create", "teacher", row.ID, nil, result, requestID); err != nil {
			return fmt.Errorf("write teacher audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return Teacher{}, mapWriteError(err)
	}
	return result, nil
}

func (s *Service) UpdateTeacher(ctx context.Context, actorID, id string, input TeacherUpdateInput, requestID string) (Teacher, error) {
	actorUUID, err := parseUUID(actorID)
	if err != nil {
		return Teacher{}, ErrUnauthorized
	}
	teacherID, err := parseUUID(id)
	if err != nil {
		return Teacher{}, ErrInvalidInput
	}
	params, userID, err := validateTeacherUpdateInput(input)
	if err != nil {
		return Teacher{}, err
	}
	params.ID = teacherID

	var result Teacher
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		existing, err := q.GetTeacherForUpdate(ctx, teacherID)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("get teacher for update: %w", err)
		}
		if userID.Valid {
			if _, err := q.GetUserByID(ctx, userID); err != nil {
				if database.IsNotFound(err) {
					return ErrNotFound
				}
				return fmt.Errorf("find teacher user: %w", err)
			}
		}
		row, err := q.UpdateTeacher(ctx, params)
		if err != nil {
			return mapWriteError(err)
		}
		result = toTeacher(row)
		name, err := employmentTypeNameByID(ctx, q, row.EmploymentTypeID)
		if err != nil {
			return err
		}
		result.EmploymentTypeName = name
		if err := s.auditData(ctx, q, actorUUID, "teacher.update", "teacher", row.ID, toTeacher(existing), result, requestID); err != nil {
			return fmt.Errorf("write teacher audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return Teacher{}, mapWriteError(err)
	}
	return result, nil
}

func (s *Service) UpdateTeacherStatus(ctx context.Context, actorID, id, status, requestID string) (Teacher, error) {
	actorUUID, err := parseUUID(actorID)
	if err != nil {
		return Teacher{}, ErrUnauthorized
	}
	teacherID, err := parseUUID(id)
	if err != nil {
		return Teacher{}, ErrInvalidInput
	}
	if !validStatus(status, "active", "inactive") {
		return Teacher{}, ErrInvalidInput
	}

	var result Teacher
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		existing, err := q.GetTeacherForUpdate(ctx, teacherID)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("get teacher for update: %w", err)
		}
		row, err := q.UpdateTeacherStatus(ctx, db.UpdateTeacherStatusParams{
			Status: status,
			ID:     teacherID,
		})
		if err != nil {
			return mapWriteError(err)
		}
		result = toTeacher(row)
		name, err := employmentTypeNameByID(ctx, q, row.EmploymentTypeID)
		if err != nil {
			return err
		}
		result.EmploymentTypeName = name
		if err := s.auditData(ctx, q, actorUUID, "teacher.status", "teacher", row.ID, toTeacher(existing), result, requestID); err != nil {
			return fmt.Errorf("write teacher status audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return Teacher{}, mapWriteError(err)
	}
	return result, nil
}

func validateTeacherInput(input TeacherInput) (db.CreateTeacherParams, pgtype.UUID, error) {
	employeeNumber, err := cleanText(input.EmployeeNumber, 50)
	if err != nil {
		return db.CreateTeacherParams{}, pgtype.UUID{}, err
	}
	fullName, err := requiredText(input.FullName, 150)
	if err != nil {
		return db.CreateTeacherParams{}, pgtype.UUID{}, err
	}
	email, err := cleanEmail(input.Email)
	if err != nil {
		return db.CreateTeacherParams{}, pgtype.UUID{}, err
	}
	phone, err := cleanText(input.Phone, 30)
	if err != nil {
		return db.CreateTeacherParams{}, pgtype.UUID{}, err
	}
	userID, err := optionalUUID(input.UserID)
	if err != nil {
		return db.CreateTeacherParams{}, pgtype.UUID{}, ErrInvalidInput
	}
	employmentTypeID, err := optionalUUID(input.EmploymentTypeID)
	if err != nil {
		return db.CreateTeacherParams{}, pgtype.UUID{}, ErrInvalidInput
	}
	status := input.Status
	if status == "" {
		status = "active"
	}
	if !validStatus(status, "active", "inactive") {
		return db.CreateTeacherParams{}, pgtype.UUID{}, ErrInvalidInput
	}
	return db.CreateTeacherParams{
		UserID:           userID,
		EmployeeNumber:   optionalText(employeeNumber),
		FullName:         fullName,
		Email:            optionalText(email),
		Phone:            optionalText(phone),
		Status:           status,
		EmploymentTypeID: employmentTypeID,
	}, userID, nil
}

func validateTeacherUpdateInput(input TeacherUpdateInput) (db.UpdateTeacherParams, pgtype.UUID, error) {
	employeeNumber, err := cleanText(input.EmployeeNumber, 50)
	if err != nil {
		return db.UpdateTeacherParams{}, pgtype.UUID{}, err
	}
	fullName, err := requiredText(input.FullName, 150)
	if err != nil {
		return db.UpdateTeacherParams{}, pgtype.UUID{}, err
	}
	email, err := cleanEmail(input.Email)
	if err != nil {
		return db.UpdateTeacherParams{}, pgtype.UUID{}, err
	}
	phone, err := cleanText(input.Phone, 30)
	if err != nil {
		return db.UpdateTeacherParams{}, pgtype.UUID{}, err
	}
	userID, err := optionalUUID(input.UserID)
	if err != nil {
		return db.UpdateTeacherParams{}, pgtype.UUID{}, ErrInvalidInput
	}
	employmentTypeID, err := optionalUUID(input.EmploymentTypeID)
	if err != nil {
		return db.UpdateTeacherParams{}, pgtype.UUID{}, ErrInvalidInput
	}
	return db.UpdateTeacherParams{
		UserID:           userID,
		EmployeeNumber:   optionalText(employeeNumber),
		FullName:         fullName,
		Email:            optionalText(email),
		Phone:            optionalText(phone),
		EmploymentTypeID: employmentTypeID,
	}, userID, nil
}

func toTeacher(row db.Teacher) Teacher {
	return Teacher{
		ID:               uuidString(row.ID),
		UserID:           uuidString(row.UserID),
		EmployeeNumber:   row.EmployeeNumber.String,
		FullName:         row.FullName,
		Email:            row.Email.String,
		Phone:            row.Phone.String,
		Status:           row.Status,
		EmploymentTypeID: uuidString(row.EmploymentTypeID),
		CreatedAt:        timestamp(row.CreatedAt),
		UpdatedAt:        timestamp(row.UpdatedAt),
	}
}

// employmentTypeNameByID resolves the display name for teachers.employment_type_id
// (employment-type package). A missing id resolves to ""; a lost row (should be
// impossible behind the FK) degrades to "" instead of failing the teacher write.
func employmentTypeNameByID(ctx context.Context, q *db.Queries, id pgtype.UUID) (string, error) {
	if !id.Valid {
		return "", nil
	}
	name, err := q.GetEmploymentTypeName(ctx, id)
	if err != nil {
		if database.IsNotFound(err) {
			return "", nil
		}
		return "", fmt.Errorf("get employment type name: %w", err)
	}
	return name, nil
}

func hasAnyEmploymentType(rows []db.Teacher) bool {
	for _, row := range rows {
		if row.EmploymentTypeID.Valid {
			return true
		}
	}
	return false
}

// employmentTypeNames loads the (small) employment_types master table once per
// page and indexes it by uuid string. Names are display-only: a load failure
// logs and yields an empty map instead of failing the list request.
func (s *Service) employmentTypeNames(ctx context.Context) map[string]string {
	rows, err := s.queries.ListEmploymentTypes(ctx)
	if err != nil {
		if s.log != nil {
			s.log.Warn("employment_type_names_load_failed", "error", err)
		}
		return map[string]string{}
	}
	names := make(map[string]string, len(rows))
	for _, row := range rows {
		names[uuidString(row.ID)] = row.Name
	}
	return names
}

func withEmploymentName(teacher Teacher, names map[string]string) Teacher {
	if teacher.EmploymentTypeID != "" {
		teacher.EmploymentTypeName = names[teacher.EmploymentTypeID]
	}
	return teacher
}
