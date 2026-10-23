package masterdata

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

func (s *Service) ListClasses(ctx context.Context, filter ClassFilter, page Page) ([]Class, int64, error) {
	page = normalizePage(page)
	params := db.ListClassesParams{
		Status:     pgtype.Text{},
		PageOffset: page.Offset,
		PageLimit:  page.Limit,
	}
	if filter.Status != "" {
		if !validStatus(filter.Status, "active", "archived") {
			return nil, 0, ErrInvalidInput
		}
		params.Status = optionalText(filter.Status)
	}
	if filter.AcademicYearID != "" {
		id, err := parseUUID(filter.AcademicYearID)
		if err != nil {
			return nil, 0, ErrInvalidInput
		}
		params.AcademicYearID = id
	}
	rows, err := s.queries.ListClasses(ctx, params)
	if err != nil {
		return nil, 0, fmt.Errorf("list classes: %w", err)
	}
	total, err := s.queries.CountClasses(ctx, db.CountClassesParams{
		AcademicYearID: params.AcademicYearID,
		Status:         params.Status,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count classes: %w", err)
	}
	classes := make([]Class, 0, len(rows))
	for _, row := range rows {
		classes = append(classes, toClassFromList(row))
	}
	return classes, total, nil
}

func (s *Service) GetClass(ctx context.Context, id string) (ClassDetail, error) {
	classID, err := parseUUID(id)
	if err != nil {
		return ClassDetail{}, ErrInvalidInput
	}
	row, err := s.queries.GetClass(ctx, classID)
	if err != nil {
		if database.IsNotFound(err) {
			return ClassDetail{}, ErrNotFound
		}
		return ClassDetail{}, fmt.Errorf("get class: %w", err)
	}
	detail := ClassDetail{Class: toClassFromGet(row)}
	homeroom, err := s.queries.GetCurrentHomeroomForClass(ctx, classID)
	if err != nil {
		if !database.IsNotFound(err) {
			return ClassDetail{}, fmt.Errorf("get current homeroom: %w", err)
		}
	} else {
		assignment := toHomeroomAssignment(homeroom)
		detail.Homeroom = &assignment
	}
	return detail, nil
}

func (s *Service) CreateClass(ctx context.Context, actorID string, input ClassInput, requestID string) (ClassDetail, error) {
	actorUUID, err := parseUUID(actorID)
	if err != nil {
		return ClassDetail{}, ErrUnauthorized
	}
	params, err := validateClassInput(input)
	if err != nil {
		return ClassDetail{}, err
	}

	var result ClassDetail
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		if _, err := q.GetAcademicYear(ctx, params.AcademicYearID); err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("find academic year: %w", err)
		}
		classID, err := q.CreateClass(ctx, params)
		if err != nil {
			return mapWriteError(err)
		}
		row, err := q.GetClass(ctx, classID)
		if err != nil {
			return fmt.Errorf("get created class: %w", err)
		}
		result = ClassDetail{Class: toClassFromGet(row)}
		if err := s.auditData(ctx, q, actorUUID, "class.create", "class", classID, nil, result.Class, requestID); err != nil {
			return fmt.Errorf("write class audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return ClassDetail{}, mapWriteError(err)
	}
	return result, nil
}

func (s *Service) UpdateClass(ctx context.Context, actorID, id string, input ClassUpdateInput, requestID string) (ClassDetail, error) {
	actorUUID, err := parseUUID(actorID)
	if err != nil {
		return ClassDetail{}, ErrUnauthorized
	}
	classID, err := parseUUID(id)
	if err != nil {
		return ClassDetail{}, ErrInvalidInput
	}
	params, err := validateClassUpdateInput(input)
	if err != nil {
		return ClassDetail{}, err
	}
	params.ID = classID

	var result ClassDetail
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		existing, err := q.GetClassForUpdate(ctx, classID)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("get class for update: %w", err)
		}
		before, err := s.getClassForAudit(ctx, q, classID)
		if err != nil {
			return err
		}
		if _, err := q.UpdateClass(ctx, params); err != nil {
			return mapWriteError(err)
		}
		result, err = getClassDetail(ctx, q, classID)
		if err != nil {
			return err
		}
		if err := s.auditData(ctx, q, actorUUID, "class.update", "class", existing.ID, before, result.Class, requestID); err != nil {
			return fmt.Errorf("write class audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return ClassDetail{}, mapWriteError(err)
	}
	return result, nil
}

func (s *Service) UpdateClassStatus(ctx context.Context, actorID, id, status, requestID string) (ClassDetail, error) {
	actorUUID, err := parseUUID(actorID)
	if err != nil {
		return ClassDetail{}, ErrUnauthorized
	}
	classID, err := parseUUID(id)
	if err != nil {
		return ClassDetail{}, ErrInvalidInput
	}
	if !validStatus(status, "active", "archived") {
		return ClassDetail{}, ErrInvalidInput
	}

	var result ClassDetail
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		existing, err := q.GetClassForUpdate(ctx, classID)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("get class for update: %w", err)
		}
		before, err := s.getClassForAudit(ctx, q, classID)
		if err != nil {
			return err
		}
		if _, err := q.UpdateClassStatus(ctx, db.UpdateClassStatusParams{Status: status, ID: classID}); err != nil {
			return mapWriteError(err)
		}
		result, err = getClassDetail(ctx, q, classID)
		if err != nil {
			return err
		}
		if err := s.auditData(ctx, q, actorUUID, "class.status", "class", existing.ID, before, result.Class, requestID); err != nil {
			return fmt.Errorf("write class status audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return ClassDetail{}, mapWriteError(err)
	}
	return result, nil
}

func (s *Service) ListClassStudents(ctx context.Context, id string) ([]ClassStudent, error) {
	classID, err := parseUUID(id)
	if err != nil {
		return nil, ErrInvalidInput
	}
	class, err := s.GetClass(ctx, id)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListClassStudents(ctx, classID)
	if err != nil {
		return nil, fmt.Errorf("list class students: %w", err)
	}
	students := make([]ClassStudent, 0, len(rows))
	for _, row := range rows {
		student := studentFromList(row)
		student.CurrentClass = &ClassRef{ID: class.ID, Name: class.Name}
		students = append(students, ClassStudent{
			Student:         student,
			EnrollmentStart: dateString(row.EnrollmentStart),
			EnrollmentEnd:   dateString(row.EnrollmentEnd),
		})
	}
	return students, nil
}

func (s *Service) AssignHomeroom(ctx context.Context, actorID, classID string, input HomeroomInput, requestID string) (HomeroomAssignment, error) {
	actorUUID, err := parseUUID(actorID)
	if err != nil {
		return HomeroomAssignment{}, ErrUnauthorized
	}
	classUUID, err := parseUUID(classID)
	if err != nil {
		return HomeroomAssignment{}, ErrInvalidInput
	}
	teacherUUID, err := parseUUID(input.TeacherID)
	if err != nil {
		return HomeroomAssignment{}, ErrInvalidInput
	}
	if input.StartDate != "" {
		if _, err := parseDate(input.StartDate); err != nil {
			return HomeroomAssignment{}, ErrInvalidInput
		}
	}

	var result HomeroomAssignment
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		class, err := q.GetClassForUpdate(ctx, classUUID)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("get class for homeroom: %w", err)
		}
		if class.Status != "active" {
			return ErrInvalidInput
		}
		teacher, err := q.GetTeacherForUpdate(ctx, teacherUUID)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("get teacher for homeroom: %w", err)
		}
		if teacher.Status != "active" {
			return ErrInvalidInput
		}
		startDate, err := resolveDate(input.StartDate, class.AcademicYearStart, class.AcademicYearEnd)
		if err != nil {
			return err
		}

		open, err := q.GetOpenHomeroomForClass(ctx, classUUID)
		if err != nil && !database.IsNotFound(err) {
			return fmt.Errorf("get open homeroom: %w", err)
		}
		var before any
		if err == nil {
			previousTeacherName := ""
			previousTeacher, teacherErr := q.GetTeacher(ctx, open.TeacherID)
			if teacherErr != nil && !database.IsNotFound(teacherErr) {
				return fmt.Errorf("get previous homeroom teacher: %w", teacherErr)
			}
			if teacherErr == nil {
				previousTeacherName = previousTeacher.FullName
			}
			before = toHomeroomAssignmentFromRow(open, previousTeacherName)
			if open.TeacherID == teacherUUID && open.StartDate.Time.Equal(startDate.Time) {
				result = toHomeroomAssignmentFromRow(open, teacher.FullName)
				return nil
			}
			if !startDate.Time.After(open.StartDate.Time) {
				return ErrConflict
			}
			endDate := startDate.Time.AddDate(0, 0, -1)
			if err := q.CloseHomeroomAssignment(ctx, db.CloseHomeroomAssignmentParams{
				EndDate: pgtype.Date{Time: endDate, Valid: true},
				ID:      open.ID,
			}); err != nil {
				return mapWriteError(err)
			}
		}

		row, err := q.CreateHomeroomAssignment(ctx, db.CreateHomeroomAssignmentParams{
			TeacherID: teacherUUID,
			ClassID:   classUUID,
			StartDate: startDate,
			CreatedBy: actorUUID,
		})
		if err != nil {
			return mapWriteError(err)
		}
		result = toHomeroomAssignmentFromRow(row, teacher.FullName)
		if err := s.auditData(ctx, q, actorUUID, "homeroom.assign", "class", classUUID, before, result, requestID); err != nil {
			return fmt.Errorf("write homeroom audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return HomeroomAssignment{}, mapWriteError(err)
	}
	return result, nil
}

func validateClassInput(input ClassInput) (db.CreateClassParams, error) {
	academicYearID, err := parseUUID(input.AcademicYearID)
	if err != nil {
		return db.CreateClassParams{}, ErrInvalidInput
	}
	name, err := requiredText(input.Name, 100)
	if err != nil {
		return db.CreateClassParams{}, err
	}
	if input.GradeLevel < 1 || input.GradeLevel > 13 {
		return db.CreateClassParams{}, ErrInvalidInput
	}
	major, err := cleanText(input.Major, 100)
	if err != nil {
		return db.CreateClassParams{}, err
	}
	return db.CreateClassParams{
		AcademicYearID: academicYearID,
		Name:           name,
		GradeLevel:     input.GradeLevel,
		Major:          optionalText(major),
	}, nil
}

func validateClassUpdateInput(input ClassUpdateInput) (db.UpdateClassParams, error) {
	name, err := requiredText(input.Name, 100)
	if err != nil {
		return db.UpdateClassParams{}, err
	}
	if input.GradeLevel < 1 || input.GradeLevel > 13 {
		return db.UpdateClassParams{}, ErrInvalidInput
	}
	major, err := cleanText(input.Major, 100)
	if err != nil {
		return db.UpdateClassParams{}, err
	}
	return db.UpdateClassParams{
		Name:       name,
		GradeLevel: input.GradeLevel,
		Major:      optionalText(major),
	}, nil
}

func (s *Service) getClassForAudit(ctx context.Context, q *db.Queries, id pgtype.UUID) (Class, error) {
	row, err := q.GetClass(ctx, id)
	if err != nil {
		if database.IsNotFound(err) {
			return Class{}, ErrNotFound
		}
		return Class{}, fmt.Errorf("get class for audit: %w", err)
	}
	return toClassFromGet(row), nil
}

func getClassDetail(ctx context.Context, q *db.Queries, id pgtype.UUID) (ClassDetail, error) {
	row, err := q.GetClass(ctx, id)
	if err != nil {
		if database.IsNotFound(err) {
			return ClassDetail{}, ErrNotFound
		}
		return ClassDetail{}, fmt.Errorf("get class detail: %w", err)
	}
	detail := ClassDetail{Class: toClassFromGet(row)}
	homeroom, err := q.GetCurrentHomeroomForClass(ctx, id)
	if err != nil {
		if !database.IsNotFound(err) {
			return ClassDetail{}, fmt.Errorf("get current homeroom: %w", err)
		}
		return detail, nil
	}
	assignment := toHomeroomAssignment(homeroom)
	detail.Homeroom = &assignment
	return detail, nil
}

func toClassFromGet(row db.GetClassRow) Class {
	return Class{
		ID:                  uuidString(row.ID),
		AcademicYearID:      uuidString(row.AcademicYearID),
		AcademicYearName:    row.AcademicYearName,
		Name:                row.Name,
		GradeLevel:          row.GradeLevel,
		Major:               row.Major.String,
		Status:              row.Status,
		HomeroomTeacherID:   uuidString(row.HomeroomTeacherID),
		HomeroomTeacherName: row.HomeroomTeacherName,
		CreatedAt:           timestamp(row.CreatedAt),
		UpdatedAt:           timestamp(row.UpdatedAt),
	}
}

func toClassFromList(row db.ListClassesRow) Class {
	return Class{
		ID:                  uuidString(row.ID),
		AcademicYearID:      uuidString(row.AcademicYearID),
		AcademicYearName:    row.AcademicYearName,
		Name:                row.Name,
		GradeLevel:          row.GradeLevel,
		Major:               row.Major.String,
		Status:              row.Status,
		HomeroomTeacherID:   uuidString(row.HomeroomTeacherID),
		HomeroomTeacherName: row.HomeroomTeacherName,
		CreatedAt:           timestamp(row.CreatedAt),
		UpdatedAt:           timestamp(row.UpdatedAt),
	}
}

func toHomeroomAssignment(row db.GetCurrentHomeroomForClassRow) HomeroomAssignment {
	return HomeroomAssignment{
		ID:          uuidString(row.ID),
		TeacherID:   uuidString(row.TeacherID),
		TeacherName: row.TeacherName,
		ClassID:     uuidString(row.ClassID),
		StartDate:   dateString(row.StartDate),
		EndDate:     dateString(row.EndDate),
		CreatedAt:   timestamp(row.CreatedAt),
	}
}

func toHomeroomAssignmentFromRow(row db.HomeroomAssignment, teacherName string) HomeroomAssignment {
	return HomeroomAssignment{
		ID:          uuidString(row.ID),
		TeacherID:   uuidString(row.TeacherID),
		TeacherName: teacherName,
		ClassID:     uuidString(row.ClassID),
		StartDate:   dateString(row.StartDate),
		EndDate:     dateString(row.EndDate),
		CreatedAt:   timestamp(row.CreatedAt),
	}
}
