package masterdata

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

func (s *Service) ListStudents(ctx context.Context, filter StudentFilter, page Page) ([]Student, int64, error) {
	page = normalizePage(page)
	sortBy := filter.SortBy
	if sortBy == "" {
		sortBy = "name"
	}
	if sortBy != "name" && sortBy != "nis" {
		return nil, 0, ErrInvalidInput
	}
	params := db.ListStudentsParams{
		Status:     pgtype.Text{},
		SortBy:     sortBy,
		PageOffset: page.Offset,
		PageLimit:  page.Limit,
	}
	if filter.Status != "" {
		if !validStatus(filter.Status, "active", "graduated", "transferred", "inactive") {
			return nil, 0, ErrInvalidInput
		}
		params.Status = optionalText(filter.Status)
	}
	if filter.ClassID != "" {
		classID, err := parseUUID(filter.ClassID)
		if err != nil {
			return nil, 0, ErrInvalidInput
		}
		params.ClassID = classID
	}
	search, err := cleanText(filter.Search, 150)
	if err != nil {
		return nil, 0, err
	}
	params.Search = optionalText(search)

	rows, err := s.queries.ListStudents(ctx, params)
	if err != nil {
		return nil, 0, fmt.Errorf("list students: %w", err)
	}
	total, err := s.queries.CountStudents(ctx, db.CountStudentsParams{
		Status:  params.Status,
		ClassID: params.ClassID,
		Search:  params.Search,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count students: %w", err)
	}
	students := make([]Student, 0, len(rows))
	for _, row := range rows {
		students = append(students, studentFromStudentsList(row))
	}
	return students, total, nil
}

func (s *Service) GetStudent(ctx context.Context, id string) (StudentDetail, error) {
	studentID, err := parseUUID(id)
	if err != nil {
		return StudentDetail{}, ErrInvalidInput
	}
	return s.getStudentDetail(ctx, s.queries, studentID)
}

func (s *Service) CreateStudent(ctx context.Context, actorID string, input StudentInput, requestID string) (StudentDetail, error) {
	actorUUID, err := parseUUID(actorID)
	if err != nil {
		return StudentDetail{}, ErrUnauthorized
	}
	params, classID, classStart, err := validateStudentInput(input)
	if err != nil {
		return StudentDetail{}, err
	}

	var result StudentDetail
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		row, err := q.CreateStudent(ctx, params)
		if err != nil {
			return mapWriteError(err)
		}
		if classID.Valid {
			class, err := q.GetClassForUpdate(ctx, classID)
			if err != nil {
				if database.IsNotFound(err) {
					return ErrNotFound
				}
				return fmt.Errorf("find student class: %w", err)
			}
			if class.Status != "active" {
				return ErrInvalidInput
			}
			startDate, err := resolveDate(classStart, class.AcademicYearStart, class.AcademicYearEnd)
			if err != nil {
				return err
			}
			if _, err := q.CreateEnrollment(ctx, db.CreateEnrollmentParams{
				StudentID: row.ID,
				ClassID:   classID,
				StartDate: startDate,
				CreatedBy: actorUUID,
			}); err != nil {
				return mapWriteError(err)
			}
		}
		result, err = s.getStudentDetail(ctx, q, row.ID)
		if err != nil {
			return err
		}
		if err := s.auditData(ctx, q, actorUUID, "student.create", "student", row.ID, nil, toStudentAudit(result, nil), requestID); err != nil {
			return fmt.Errorf("write student audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return StudentDetail{}, mapWriteError(err)
	}
	return result, nil
}

func (s *Service) UpdateStudent(ctx context.Context, actorID, id string, input StudentUpdateInput, requestID string) (StudentDetail, error) {
	actorUUID, err := parseUUID(actorID)
	if err != nil {
		return StudentDetail{}, ErrUnauthorized
	}
	studentID, err := parseUUID(id)
	if err != nil {
		return StudentDetail{}, ErrInvalidInput
	}
	params, err := validateStudentUpdateInput(input)
	if err != nil {
		return StudentDetail{}, err
	}
	params.ID = studentID

	var result StudentDetail
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		if _, err := q.GetStudentForUpdate(ctx, studentID); err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("get student for update: %w", err)
		}
		before, err := s.getStudentDetail(ctx, q, studentID)
		if err != nil {
			return err
		}
		if _, err := q.UpdateStudent(ctx, params); err != nil {
			return mapWriteError(err)
		}
		result, err = s.getStudentDetail(ctx, q, studentID)
		if err != nil {
			return err
		}
		if err := s.auditData(ctx, q, actorUUID, "student.update", "student", studentID, toStudentAudit(before, nil), toStudentAudit(result, studentChangedFields(before, result)), requestID); err != nil {
			return fmt.Errorf("write student audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return StudentDetail{}, mapWriteError(err)
	}
	return result, nil
}

func (s *Service) UpdateStudentStatus(ctx context.Context, actorID, id, status, requestID string) (StudentDetail, error) {
	actorUUID, err := parseUUID(actorID)
	if err != nil {
		return StudentDetail{}, ErrUnauthorized
	}
	studentID, err := parseUUID(id)
	if err != nil {
		return StudentDetail{}, ErrInvalidInput
	}
	if !validStatus(status, "active", "graduated", "transferred", "inactive") {
		return StudentDetail{}, ErrInvalidInput
	}

	var result StudentDetail
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		if _, err := q.GetStudentForUpdate(ctx, studentID); err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("get student for update: %w", err)
		}
		before, err := s.getStudentDetail(ctx, q, studentID)
		if err != nil {
			return err
		}
		if status != "active" {
			open, openErr := q.GetOpenEnrollmentForStudentForUpdate(ctx, studentID)
			if openErr == nil {
				endDate := todayDate()
				if endDate.Before(open.StartDate.Time) {
					endDate = open.StartDate.Time
				}
				year, yearErr := q.GetClassYearDates(ctx, open.ClassID)
				if yearErr != nil {
					return fmt.Errorf("get enrollment class year: %w", yearErr)
				}
				if endDate.After(year.EndDate.Time) {
					endDate = year.EndDate.Time
				}
				if endDate.Before(open.StartDate.Time) {
					endDate = open.StartDate.Time
				}
				if err := q.CloseEnrollment(ctx, db.CloseEnrollmentParams{
					EndDate: pgtype.Date{Time: endDate, Valid: true},
					ID:      open.ID,
				}); err != nil {
					return mapWriteError(err)
				}
			} else if !database.IsNotFound(openErr) {
				return fmt.Errorf("get open enrollment: %w", openErr)
			}
		}
		if _, err := q.UpdateStudentStatus(ctx, db.UpdateStudentStatusParams{
			Status: status,
			ID:     studentID,
		}); err != nil {
			return mapWriteError(err)
		}
		result, err = s.getStudentDetail(ctx, q, studentID)
		if err != nil {
			return err
		}
		if err := s.auditData(ctx, q, actorUUID, "student.status", "student", studentID, toStudentAudit(before, nil), toStudentAudit(result, studentChangedFields(before, result)), requestID); err != nil {
			return fmt.Errorf("write student status audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return StudentDetail{}, mapWriteError(err)
	}
	return result, nil
}

func (s *Service) AssignStudentClass(ctx context.Context, actorID, id string, input StudentClassInput, requestID string) (StudentDetail, error) {
	actorUUID, err := parseUUID(actorID)
	if err != nil {
		return StudentDetail{}, ErrUnauthorized
	}
	studentID, err := parseUUID(id)
	if err != nil {
		return StudentDetail{}, ErrInvalidInput
	}
	classID, err := parseUUID(input.ClassID)
	if err != nil {
		return StudentDetail{}, ErrInvalidInput
	}
	if input.StartDate != "" {
		if _, err := parseDate(input.StartDate); err != nil {
			return StudentDetail{}, ErrInvalidInput
		}
	}

	var result StudentDetail
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		student, err := q.GetStudentForUpdate(ctx, studentID)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("get student for class assignment: %w", err)
		}
		if student.Status != "active" {
			return ErrInvalidInput
		}
		class, err := q.GetClassForUpdate(ctx, classID)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("get target class: %w", err)
		}
		if class.Status != "active" {
			return ErrInvalidInput
		}
		startDate, err := resolveDate(input.StartDate, class.AcademicYearStart, class.AcademicYearEnd)
		if err != nil {
			return err
		}

		before := map[string]string{}
		open, err := q.GetOpenEnrollmentForStudentForUpdate(ctx, studentID)
		if err != nil && !database.IsNotFound(err) {
			return fmt.Errorf("get open enrollment: %w", err)
		}
		if err == nil {
			if open.ClassID == classID {
				result, err = s.getStudentDetail(ctx, q, studentID)
				return err
			}
			if !startDate.Time.After(open.StartDate.Time) {
				return ErrConflict
			}
			previousYear, err := q.GetClassYearDates(ctx, open.ClassID)
			if err != nil {
				return fmt.Errorf("get previous class year: %w", err)
			}
			endDate := startDate.Time.AddDate(0, 0, -1)
			if endDate.After(previousYear.EndDate.Time) {
				endDate = previousYear.EndDate.Time
			}
			if endDate.Before(open.StartDate.Time) {
				return ErrConflict
			}
			if err := q.CloseEnrollment(ctx, db.CloseEnrollmentParams{
				EndDate: pgtype.Date{Time: endDate, Valid: true},
				ID:      open.ID,
			}); err != nil {
				return mapWriteError(err)
			}
			before = map[string]string{
				"class_id":   uuidString(open.ClassID),
				"start_date": dateString(open.StartDate),
				"end_date":   dateString(pgtype.Date{Time: endDate, Valid: true}),
			}
		}

		if _, err := q.CreateEnrollment(ctx, db.CreateEnrollmentParams{
			StudentID: studentID,
			ClassID:   classID,
			StartDate: startDate,
			CreatedBy: actorUUID,
		}); err != nil {
			return mapWriteError(err)
		}
		result, err = s.getStudentDetail(ctx, q, studentID)
		if err != nil {
			return err
		}
		after := map[string]string{
			"class_id":   uuidString(classID),
			"start_date": dateString(startDate),
		}
		if err := s.auditData(ctx, q, actorUUID, "student.class_assign", "student", studentID, before, after, requestID); err != nil {
			return fmt.Errorf("write student class audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return StudentDetail{}, mapWriteError(err)
	}
	return result, nil
}

func (s *Service) getStudentDetail(ctx context.Context, q *db.Queries, id pgtype.UUID) (StudentDetail, error) {
	row, err := q.GetStudent(ctx, id)
	if err != nil {
		if database.IsNotFound(err) {
			return StudentDetail{}, ErrNotFound
		}
		return StudentDetail{}, fmt.Errorf("get student: %w", err)
	}
	enrollments, err := q.ListStudentEnrollments(ctx, id)
	if err != nil {
		return StudentDetail{}, fmt.Errorf("list student enrollments: %w", err)
	}
	result := StudentDetail{
		Student:     studentFromGet(row),
		Enrollments: make([]Enrollment, 0, len(enrollments)),
	}
	for _, enrollment := range enrollments {
		result.Enrollments = append(result.Enrollments, Enrollment{
			ID:               uuidString(enrollment.ID),
			ClassID:          uuidString(enrollment.ClassID),
			ClassName:        enrollment.ClassName,
			AcademicYearID:   uuidString(enrollment.AcademicYearID),
			AcademicYearName: enrollment.AcademicYearName,
			StartDate:        dateString(enrollment.StartDate),
			EndDate:          dateString(enrollment.EndDate),
		})
	}
	return result, nil
}

type studentAudit struct {
	ID                    string                   `json:"id"`
	StudentNumber         string                   `json:"student_number"`
	NationalStudentNumber string                   `json:"national_student_number,omitempty"`
	FullName              string                   `json:"full_name"`
	Gender                string                   `json:"gender,omitempty"`
	AdmissionYear         int16                    `json:"admission_year"`
	Status                string                   `json:"status"`
	CurrentClassID        string                   `json:"current_class_id,omitempty"`
	Enrollments           []studentEnrollmentAudit `json:"enrollments,omitempty"`
	ChangedFields         []string                 `json:"changed_fields,omitempty"`
}

type studentEnrollmentAudit struct {
	ClassID   string `json:"class_id"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date,omitempty"`
}

func toStudentAudit(detail StudentDetail, changed []string) studentAudit {
	audit := studentAudit{
		ID:                    detail.ID,
		StudentNumber:         detail.StudentNumber,
		NationalStudentNumber: detail.NationalStudentNumber,
		FullName:              detail.FullName,
		Gender:                detail.Gender,
		AdmissionYear:         detail.AdmissionYear,
		Status:                detail.Status,
		ChangedFields:         changed,
		Enrollments:           make([]studentEnrollmentAudit, 0, len(detail.Enrollments)),
	}
	if detail.CurrentClass != nil {
		audit.CurrentClassID = detail.CurrentClass.ID
	}
	for _, enrollment := range detail.Enrollments {
		audit.Enrollments = append(audit.Enrollments, studentEnrollmentAudit{
			ClassID:   enrollment.ClassID,
			StartDate: enrollment.StartDate,
			EndDate:   enrollment.EndDate,
		})
	}
	return audit
}

func studentChangedFields(before, after StudentDetail) []string {
	changed := make([]string, 0, 10)
	if before.StudentNumber != after.StudentNumber {
		changed = append(changed, "student_number")
	}
	if before.NationalStudentNumber != after.NationalStudentNumber {
		changed = append(changed, "national_student_number")
	}
	if before.FullName != after.FullName {
		changed = append(changed, "full_name")
	}
	if before.Gender != after.Gender {
		changed = append(changed, "gender")
	}
	if before.BirthDate != after.BirthDate {
		changed = append(changed, "birth_date")
	}
	if before.AdmissionYear != after.AdmissionYear {
		changed = append(changed, "admission_year")
	}
	if before.Status != after.Status {
		changed = append(changed, "status")
	}
	if before.GuardianName != after.GuardianName {
		changed = append(changed, "guardian_name")
	}
	if before.GuardianPhone != after.GuardianPhone {
		changed = append(changed, "guardian_phone")
	}
	beforeClassID, afterClassID := "", ""
	if before.CurrentClass != nil {
		beforeClassID = before.CurrentClass.ID
	}
	if after.CurrentClass != nil {
		afterClassID = after.CurrentClass.ID
	}
	if beforeClassID != afterClassID {
		changed = append(changed, "current_class")
	}
	if len(before.Enrollments) != len(after.Enrollments) {
		changed = append(changed, "enrollments")
	} else {
		for i := range before.Enrollments {
			if before.Enrollments[i] != after.Enrollments[i] {
				changed = append(changed, "enrollments")
				break
			}
		}
	}
	return changed
}

func validateStudentInput(input StudentInput) (db.CreateStudentParams, pgtype.UUID, string, error) {
	studentNumber, err := requiredText(input.StudentNumber, 50)
	if err != nil {
		return db.CreateStudentParams{}, pgtype.UUID{}, "", err
	}
	nisn, err := cleanText(input.NationalStudentNumber, 20)
	if err != nil {
		return db.CreateStudentParams{}, pgtype.UUID{}, "", err
	}
	fullName, err := requiredText(input.FullName, 150)
	if err != nil {
		return db.CreateStudentParams{}, pgtype.UUID{}, "", err
	}
	gender, err := cleanText(input.Gender, 1)
	if err != nil || (gender != "" && !validStatus(gender, "L", "P")) {
		return db.CreateStudentParams{}, pgtype.UUID{}, "", ErrInvalidInput
	}
	birthDate, err := optionalDate(input.BirthDate)
	if err != nil {
		return db.CreateStudentParams{}, pgtype.UUID{}, "", ErrInvalidInput
	}
	if input.AdmissionYear < 1990 || input.AdmissionYear > 2100 {
		return db.CreateStudentParams{}, pgtype.UUID{}, "", ErrInvalidInput
	}
	status := input.Status
	if status == "" {
		status = "active"
	}
	if !validStatus(status, "active", "graduated", "transferred", "inactive") {
		return db.CreateStudentParams{}, pgtype.UUID{}, "", ErrInvalidInput
	}
	guardianName, err := cleanText(input.GuardianName, 150)
	if err != nil {
		return db.CreateStudentParams{}, pgtype.UUID{}, "", err
	}
	guardianPhone, err := cleanText(input.GuardianPhone, 30)
	if err != nil {
		return db.CreateStudentParams{}, pgtype.UUID{}, "", err
	}
	classID, err := optionalUUID(input.ClassID)
	if err != nil {
		return db.CreateStudentParams{}, pgtype.UUID{}, "", ErrInvalidInput
	}
	if classID.Valid && status != "active" {
		return db.CreateStudentParams{}, pgtype.UUID{}, "", ErrInvalidInput
	}
	if input.ClassStartDate != "" {
		if !classID.Valid {
			return db.CreateStudentParams{}, pgtype.UUID{}, "", ErrInvalidInput
		}
		if _, err := parseDate(input.ClassStartDate); err != nil {
			return db.CreateStudentParams{}, pgtype.UUID{}, "", ErrInvalidInput
		}
	}
	return db.CreateStudentParams{
		StudentNumber:         studentNumber,
		NationalStudentNumber: optionalText(nisn),
		FullName:              fullName,
		Gender:                optionalText(gender),
		BirthDate:             birthDate,
		AdmissionYear:         input.AdmissionYear,
		Status:                status,
		GuardianName:          optionalText(guardianName),
		GuardianPhone:         optionalText(guardianPhone),
	}, classID, input.ClassStartDate, nil
}

func validateStudentUpdateInput(input StudentUpdateInput) (db.UpdateStudentParams, error) {
	studentNumber, err := requiredText(input.StudentNumber, 50)
	if err != nil {
		return db.UpdateStudentParams{}, err
	}
	nisn, err := cleanText(input.NationalStudentNumber, 20)
	if err != nil {
		return db.UpdateStudentParams{}, err
	}
	fullName, err := requiredText(input.FullName, 150)
	if err != nil {
		return db.UpdateStudentParams{}, err
	}
	gender, err := cleanText(input.Gender, 1)
	if err != nil || (gender != "" && !validStatus(gender, "L", "P")) {
		return db.UpdateStudentParams{}, ErrInvalidInput
	}
	birthDate, err := optionalDate(input.BirthDate)
	if err != nil {
		return db.UpdateStudentParams{}, ErrInvalidInput
	}
	if input.AdmissionYear < 1990 || input.AdmissionYear > 2100 {
		return db.UpdateStudentParams{}, ErrInvalidInput
	}
	guardianName, err := cleanText(input.GuardianName, 150)
	if err != nil {
		return db.UpdateStudentParams{}, err
	}
	guardianPhone, err := cleanText(input.GuardianPhone, 30)
	if err != nil {
		return db.UpdateStudentParams{}, err
	}
	return db.UpdateStudentParams{
		StudentNumber:         studentNumber,
		NationalStudentNumber: optionalText(nisn),
		FullName:              fullName,
		Gender:                optionalText(gender),
		BirthDate:             birthDate,
		AdmissionYear:         input.AdmissionYear,
		GuardianName:          optionalText(guardianName),
		GuardianPhone:         optionalText(guardianPhone),
	}, nil
}

func studentFromGet(row db.GetStudentRow) Student {
	student := Student{
		ID:                    uuidString(row.ID),
		StudentNumber:         row.StudentNumber,
		NationalStudentNumber: row.NationalStudentNumber.String,
		FullName:              row.FullName,
		Gender:                row.Gender.String,
		BirthDate:             dateString(row.BirthDate),
		AdmissionYear:         row.AdmissionYear,
		Status:                row.Status,
		GuardianName:          row.GuardianName.String,
		GuardianPhone:         row.GuardianPhone.String,
		CreatedAt:             timestamp(row.CreatedAt),
		UpdatedAt:             timestamp(row.UpdatedAt),
	}
	if row.CurrentClassID.Valid {
		student.CurrentClass = &ClassRef{ID: uuidString(row.CurrentClassID), Name: row.CurrentClassName}
	}
	return student
}

func studentFromStudentsList(row db.ListStudentsRow) Student {
	student := Student{
		ID:                    uuidString(row.ID),
		StudentNumber:         row.StudentNumber,
		NationalStudentNumber: row.NationalStudentNumber.String,
		FullName:              row.FullName,
		Gender:                row.Gender.String,
		BirthDate:             dateString(row.BirthDate),
		AdmissionYear:         row.AdmissionYear,
		Status:                row.Status,
		GuardianName:          row.GuardianName.String,
		GuardianPhone:         row.GuardianPhone.String,
		CreatedAt:             timestamp(row.CreatedAt),
		UpdatedAt:             timestamp(row.UpdatedAt),
	}
	if row.CurrentClassID.Valid {
		student.CurrentClass = &ClassRef{ID: uuidString(row.CurrentClassID), Name: row.CurrentClassName}
	}
	return student
}

func studentFromList(row db.ListClassStudentsRow) Student {
	return Student{
		ID:                    uuidString(row.ID),
		StudentNumber:         row.StudentNumber,
		NationalStudentNumber: row.NationalStudentNumber.String,
		FullName:              row.FullName,
		Gender:                row.Gender.String,
		BirthDate:             dateString(row.BirthDate),
		AdmissionYear:         row.AdmissionYear,
		Status:                row.Status,
		GuardianName:          row.GuardianName.String,
		GuardianPhone:         row.GuardianPhone.String,
		CreatedAt:             timestamp(row.CreatedAt),
		UpdatedAt:             timestamp(row.UpdatedAt),
	}
}
