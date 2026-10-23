package imports

import (
	"bytes"
	"context"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/xuri/excelize/v2"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

var studentHeaders = map[string][]string{
	"nis":            {"NIS"},
	"nisn":           {"NISN"},
	"name":           {"Nama Lengkap", "Nama"},
	"gender":         {"Jenis Kelamin", "Gender"},
	"birth_date":     {"Tanggal Lahir"},
	"admission_year": {"Tahun Masuk"},
	"status":         {"Status"},
	"guardian_name":  {"Nama Wali"},
	"guardian_phone": {"Telepon Wali", "No Telepon Wali"},
	"academic_year":  {"Tahun Ajaran"},
	"class":          {"Kelas"},
	"class_start":    {"Tanggal Mulai Kelas"},
}

type studentInput struct {
	Row                   int
	StudentNumber         string
	NationalStudentNumber string
	FullName              string
	Gender                string
	BirthDate             pgtype.Date
	BirthDateRaw          string
	AdmissionYear         int16
	AdmissionYearRaw      string
	Status                string
	GuardianName          string
	GuardianPhone         string
	AcademicYear          string
	ClassName             string
	ClassStart            string
}

type studentImportRow struct {
	Row             int
	Action          string
	StudentNumber   string
	FullName        string
	ClassName       string
	Changed         []string
	ExistingID      pgtype.UUID
	Expected        *db.Student
	Create          db.CreateStudentParams
	Update          db.UpdateStudentParams
	TargetStatus    string
	StatusUpdate    bool
	CloseEnrollment bool
	Enroll          bool
	ClassID         pgtype.UUID
	StartDate       pgtype.Date
}

func parseStudentWorkbook(data []byte, maxRows int) ([]studentInput, *RowError) {
	file, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, &RowError{RowNumber: 0, Column: "file", Code: "invalid_file", Message: "file bukan berkas Excel yang valid"}
	}
	defer func() { _ = file.Close() }()
	sheets := file.GetSheetList()
	if len(sheets) == 0 {
		return nil, &RowError{RowNumber: 0, Column: "file", Code: "empty_file", Message: "file tidak memiliki worksheet"}
	}
	rows, err := file.GetRows(sheets[0])
	if err != nil {
		return nil, &RowError{RowNumber: 0, Column: "file", Code: "invalid_file", Message: "worksheet tidak dapat dibaca"}
	}
	if len(rows) == 0 {
		return nil, &RowError{RowNumber: 0, Column: "file", Code: "empty_file", Message: "file tidak memiliki header"}
	}
	columns, missing := columnMap(rows[0], studentHeaders)
	if missing != "" {
		return nil, &RowError{RowNumber: 1, Column: missing, Code: "missing_header", Message: "kolom wajib tidak ditemukan pada header"}
	}

	inputs := make([]studentInput, 0, len(rows)-1)
	for index := 1; index < len(rows); index++ {
		row := rows[index]
		if rowBlank(row) {
			continue
		}
		input := studentInput{
			Row:                   index + 1,
			StudentNumber:         cell(row, columns, "nis"),
			NationalStudentNumber: cell(row, columns, "nisn"),
			FullName:              cell(row, columns, "name"),
			Gender:                cell(row, columns, "gender"),
			Status:                cell(row, columns, "status"),
			GuardianName:          cell(row, columns, "guardian_name"),
			GuardianPhone:         cell(row, columns, "guardian_phone"),
			AcademicYear:          cell(row, columns, "academic_year"),
			ClassName:             cell(row, columns, "class"),
			ClassStart:            cell(row, columns, "class_start"),
		}
		if value := cell(row, columns, "birth_date"); value != "" {
			input.BirthDateRaw = value
			parsed, parseErr := parseDateCell(value)
			if parseErr == nil && parsed.Valid {
				input.BirthDate = parsed
			}
		}
		if value := cell(row, columns, "admission_year"); value != "" {
			input.AdmissionYearRaw = value
			parsed, parseErr := strconv.ParseInt(value, 10, 16)
			if parseErr == nil {
				input.AdmissionYear = int16(parsed)
			}
		}
		inputs = append(inputs, input)
	}
	if len(inputs) == 0 {
		return nil, &RowError{RowNumber: 0, Column: "file", Code: "empty_file", Message: "file tidak memiliki baris data"}
	}
	if len(inputs) > maxRows {
		return nil, &RowError{RowNumber: 0, Column: "file", Code: "too_many_rows", Message: "jumlah baris melebihi batas " + strconv.Itoa(maxRows)}
	}
	return inputs, nil
}

func (s *Service) validateStudents(ctx context.Context, q *db.Queries, mode Mode, inputs []studentInput) (Validation, error) {
	result := Validation{}
	seen := make(map[string]int, len(inputs))
	seenNISN := make(map[string]int, len(inputs))

	for _, input := range inputs {
		result.TotalRows++
		rowErrors := make([]RowError, 0, 2)
		addError := func(column, code, message string) {
			rowErrors = append(rowErrors, RowError{
				RowNumber: input.Row,
				Column:    column,
				Code:      code,
				Message:   message,
			})
		}

		studentNumber, err := requiredText(input.StudentNumber, 50)
		if err != nil {
			addError("NIS", "required", "NIS wajib diisi dan maksimal 50 karakter")
		}
		fullName, err := cleanText(input.FullName, 150)
		if err != nil {
			addError("Nama Lengkap", "invalid", "nama maksimal 150 karakter")
		}
		nisn, err := cleanText(input.NationalStudentNumber, 20)
		if err != nil {
			addError("NISN", "invalid", "NISN maksimal 20 karakter")
		}
		gender, err := normalizeGender(input.Gender)
		if err != nil {
			addError("Jenis Kelamin", "invalid", "jenis kelamin harus L atau P")
		}
		status, err := normalizeStudentStatus(input.Status)
		if err != nil {
			addError("Status", "invalid", "status harus active, graduated, transferred, atau inactive")
		}
		guardianName, err := cleanText(input.GuardianName, 150)
		if err != nil {
			addError("Nama Wali", "invalid", "nama wali maksimal 150 karakter")
		}
		guardianPhone, err := cleanText(input.GuardianPhone, 30)
		if err != nil {
			addError("Telepon Wali", "invalid", "telepon wali maksimal 30 karakter")
		}
		if input.AdmissionYearRaw != "" && (input.AdmissionYear < 1990 || input.AdmissionYear > 2100) {
			addError("Tahun Masuk", "invalid", "tahun masuk harus antara 1990 dan 2100")
		}
		if input.BirthDateRaw != "" && !input.BirthDate.Valid {
			addError("Tanggal Lahir", "invalid", "tanggal lahir tidak valid")
		}

		className := strings.TrimSpace(input.ClassName)
		academicYear := strings.TrimSpace(input.AcademicYear)
		if className == "" {
			if academicYear != "" {
				addError("Tahun Ajaran", "invalid", "tahun ajaran diisi tanpa kelas")
			}
			if strings.TrimSpace(input.ClassStart) != "" {
				addError("Tanggal Mulai Kelas", "invalid", "tanggal mulai kelas diisi tanpa kelas")
			}
		}

		if len(rowErrors) > 0 {
			result.InvalidRows++
			result.Errors = append(result.Errors, rowErrors...)
			result.Rows = append(result.Rows, preview(input.Row, "invalid", studentNumber, fullName, className, nil))
			continue
		}

		if first, exists := seen[studentNumber]; exists {
			result.DuplicateRows++
			result.Errors = append(result.Errors, RowError{
				RowNumber: input.Row,
				Column:    "NIS",
				Code:      "duplicate_in_file",
				Message:   "NIS duplikat dengan baris " + strconv.Itoa(first),
			})
			result.Rows = append(result.Rows, preview(input.Row, "duplicate", studentNumber, fullName, className, nil))
			continue
		}
		seen[studentNumber] = input.Row

		if nisn != "" {
			if first, exists := seenNISN[nisn]; exists {
				result.InvalidRows++
				result.Errors = append(result.Errors, RowError{
					RowNumber: input.Row,
					Column:    "NISN",
					Code:      "duplicate_in_file",
					Message:   "NISN duplikat dengan baris " + strconv.Itoa(first),
				})
				result.Rows = append(result.Rows, preview(input.Row, "invalid", studentNumber, fullName, className, nil))
				continue
			}
		}

		existing, err := q.GetStudentByStudentNumber(ctx, studentNumber)
		if err != nil && !database.IsNotFound(err) {
			return Validation{}, err
		}
		var existingStudent *db.Student
		if err == nil {
			existingStudent = &existing
		}

		if nisn != "" && existingStudent != nil && existingStudent.NationalStudentNumber.Valid && existingStudent.NationalStudentNumber.String == nisn {
			// same student, no conflict
		} else if nisn != "" {
			conflict, err := q.GetStudentByNationalStudentNumber(ctx, optionalText(nisn))
			if err != nil && !database.IsNotFound(err) {
				return Validation{}, err
			}
			if err == nil && (existingStudent == nil || conflict.ID != existingStudent.ID) {
				result.InvalidRows++
				result.Errors = append(result.Errors, RowError{
					RowNumber: input.Row,
					Column:    "NISN",
					Code:      "nisn_exists",
					Message:   "NISN sudah digunakan siswa lain",
				})
				result.Rows = append(result.Rows, preview(input.Row, "invalid", studentNumber, fullName, className, nil))
				continue
			}
		}
		if nisn != "" {
			seenNISN[nisn] = input.Row
		}

		if existingStudent != nil && mode == ModeCreateOnly {
			result.DuplicateRows++
			result.Errors = append(result.Errors, RowError{
				RowNumber: input.Row,
				Column:    "NIS",
				Code:      "duplicate_nis",
				Message:   "NIS sudah terdaftar; gunakan mode upsert untuk memperbarui data",
			})
			result.Rows = append(result.Rows, preview(input.Row, "duplicate", studentNumber, fullName, className, nil))
			continue
		}

		targetStatus := status
		if targetStatus == "" {
			if existingStudent != nil {
				targetStatus = existingStudent.Status
			} else {
				targetStatus = "active"
			}
		}
		if targetStatus != "active" && className != "" {
			result.InvalidRows++
			result.Errors = append(result.Errors, RowError{
				RowNumber: input.Row,
				Column:    "Kelas",
				Code:      "inactive_with_class",
				Message:   "siswa dengan status selain active tidak boleh memiliki kelas",
			})
			result.Rows = append(result.Rows, preview(input.Row, "invalid", studentNumber, fullName, className, nil))
			continue
		}

		var classRow db.GetClassByYearAndNameRow
		hasClass := className != ""
		if hasClass {
			yearName := academicYear
			if yearName == "" {
				active, err := q.GetActiveAcademicYear(ctx)
				if err != nil {
					if database.IsNotFound(err) {
						result.InvalidRows++
						result.Errors = append(result.Errors, RowError{
							RowNumber: input.Row,
							Column:    "Tahun Ajaran",
							Code:      "active_year_missing",
							Message:   "tahun ajaran aktif belum ditetapkan",
						})
						result.Rows = append(result.Rows, preview(input.Row, "invalid", studentNumber, fullName, className, nil))
						continue
					}
					return Validation{}, err
				}
				yearName = active.Name
			}
			classRow, err = q.GetClassByYearAndName(ctx, db.GetClassByYearAndNameParams{
				AcademicYearName: yearName,
				ClassName:        className,
			})
			if err != nil {
				if database.IsNotFound(err) {
					result.InvalidRows++
					result.Errors = append(result.Errors, RowError{
						RowNumber: input.Row,
						Column:    "Kelas",
						Code:      "class_not_found",
						Message:   "kelas tidak ditemukan pada tahun ajaran yang dipilih",
					})
					result.Rows = append(result.Rows, preview(input.Row, "invalid", studentNumber, fullName, className, nil))
					continue
				}
				return Validation{}, err
			}
			if classRow.Status != "active" {
				result.InvalidRows++
				result.Errors = append(result.Errors, RowError{
					RowNumber: input.Row,
					Column:    "Kelas",
					Code:      "class_inactive",
					Message:   "kelas tidak aktif",
				})
				result.Rows = append(result.Rows, preview(input.Row, "invalid", studentNumber, fullName, className, nil))
				continue
			}
		}

		if existingStudent == nil {
			if fullName == "" {
				addError("Nama Lengkap", "required", "nama wajib diisi")
			}
			if input.AdmissionYearRaw == "" || input.AdmissionYear < 1990 || input.AdmissionYear > 2100 {
				addError("Tahun Masuk", "required", "tahun masuk wajib diisi antara 1990 dan 2100")
			}
			if len(rowErrors) > 0 {
				result.InvalidRows++
				result.Errors = append(result.Errors, rowErrors...)
				result.Rows = append(result.Rows, preview(input.Row, "invalid", studentNumber, fullName, className, nil))
				continue
			}
			startDate := pgtype.Date{}
			if hasClass {
				startDate, err = resolveDate(input.ClassStart, classRow.AcademicYearStart, classRow.AcademicYearEnd)
				if err != nil {
					result.InvalidRows++
					result.Errors = append(result.Errors, RowError{
						RowNumber: input.Row,
						Column:    "Tanggal Mulai Kelas",
						Code:      "invalid_class_start",
						Message:   "tanggal mulai kelas harus di dalam rentang tahun ajaran",
					})
					result.Rows = append(result.Rows, preview(input.Row, "invalid", studentNumber, fullName, className, nil))
					continue
				}
			}
			result.ValidRows++
			result.studentRows = append(result.studentRows, studentImportRow{
				Row:           input.Row,
				Action:        "create",
				StudentNumber: studentNumber,
				FullName:      fullName,
				ClassName:     className,
				Create: db.CreateStudentParams{
					StudentNumber:         studentNumber,
					NationalStudentNumber: optionalText(nisn),
					FullName:              fullName,
					Gender:                optionalText(gender),
					BirthDate:             input.BirthDate,
					AdmissionYear:         input.AdmissionYear,
					Status:                targetStatus,
					GuardianName:          optionalText(guardianName),
					GuardianPhone:         optionalText(guardianPhone),
				},
				Enroll:    hasClass,
				ClassID:   classRow.ID,
				StartDate: startDate,
			})
			result.Rows = append(result.Rows, preview(input.Row, "create", studentNumber, fullName, className, nil))
			continue
		}

		mergedName := existingStudent.FullName
		if fullName != "" {
			mergedName = fullName
		}
		mergedAdmissionYear := existingStudent.AdmissionYear
		if input.AdmissionYearRaw != "" {
			mergedAdmissionYear = input.AdmissionYear
		}
		mergedNISN := existingStudent.NationalStudentNumber
		if nisn != "" {
			mergedNISN = optionalText(nisn)
		}
		mergedGender := existingStudent.Gender
		if gender != "" {
			mergedGender = optionalText(gender)
		}
		mergedBirthDate := existingStudent.BirthDate
		if input.BirthDate.Valid {
			mergedBirthDate = input.BirthDate
		}
		mergedGuardianName := existingStudent.GuardianName
		if guardianName != "" {
			mergedGuardianName = optionalText(guardianName)
		}
		mergedGuardianPhone := existingStudent.GuardianPhone
		if guardianPhone != "" {
			mergedGuardianPhone = optionalText(guardianPhone)
		}

		changed := make([]string, 0, 6)
		if !sameText(mergedNISN, existingStudent.NationalStudentNumber) {
			changed = append(changed, "nisn")
		}
		if mergedName != existingStudent.FullName {
			changed = append(changed, "name")
		}
		if !sameText(mergedGender, existingStudent.Gender) {
			changed = append(changed, "gender")
		}
		if dateString(mergedBirthDate) != dateString(existingStudent.BirthDate) {
			changed = append(changed, "birth_date")
		}
		if mergedAdmissionYear != existingStudent.AdmissionYear {
			changed = append(changed, "admission_year")
		}
		if !sameText(mergedGuardianName, existingStudent.GuardianName) {
			changed = append(changed, "guardian_name")
		}
		if !sameText(mergedGuardianPhone, existingStudent.GuardianPhone) {
			changed = append(changed, "guardian_phone")
		}
		statusUpdate := status != "" && status != existingStudent.Status
		if statusUpdate {
			changed = append(changed, "status")
		}

		closeEnrollment := false
		enroll := false
		startDate := pgtype.Date{}
		open, err := q.GetOpenEnrollmentForStudent(ctx, existingStudent.ID)
		if err != nil && !database.IsNotFound(err) {
			return Validation{}, err
		}
		hasOpen := err == nil
		if hasClass {
			if hasOpen {
				if open.ClassID != classRow.ID {
					result.InvalidRows++
					result.Errors = append(result.Errors, RowError{
						RowNumber: input.Row,
						Column:    "Kelas",
						Code:      "student_has_class",
						Message:   "siswa sudah terdaftar di kelas lain; gunakan pindah kelas",
					})
					result.Rows = append(result.Rows, preview(input.Row, "invalid", studentNumber, fullName, className, nil))
					continue
				}
			} else {
				startDate, err = resolveDate(input.ClassStart, classRow.AcademicYearStart, classRow.AcademicYearEnd)
				if err != nil {
					result.InvalidRows++
					result.Errors = append(result.Errors, RowError{
						RowNumber: input.Row,
						Column:    "Tanggal Mulai Kelas",
						Code:      "invalid_class_start",
						Message:   "tanggal mulai kelas harus di dalam rentang tahun ajaran",
					})
					result.Rows = append(result.Rows, preview(input.Row, "invalid", studentNumber, fullName, className, nil))
					continue
				}
				enroll = true
				changed = append(changed, "class")
			}
		} else if targetStatus != "active" && hasOpen {
			closeEnrollment = true
			changed = append(changed, "class")
		}

		action := "update"
		if len(changed) == 0 && !statusUpdate && !enroll && !closeEnrollment {
			action = "unchanged"
		}
		result.ValidRows++
		result.studentRows = append(result.studentRows, studentImportRow{
			Row:           input.Row,
			Action:        action,
			StudentNumber: studentNumber,
			FullName:      mergedName,
			ClassName:     className,
			Changed:       changed,
			ExistingID:    existingStudent.ID,
			Expected:      existingStudent,
			Update: db.UpdateStudentParams{
				StudentNumber:         studentNumber,
				NationalStudentNumber: mergedNISN,
				FullName:              mergedName,
				Gender:                mergedGender,
				BirthDate:             mergedBirthDate,
				AdmissionYear:         mergedAdmissionYear,
				GuardianName:          mergedGuardianName,
				GuardianPhone:         mergedGuardianPhone,
				ID:                    existingStudent.ID,
			},
			TargetStatus:    targetStatus,
			StatusUpdate:    statusUpdate,
			CloseEnrollment: closeEnrollment,
			Enroll:          enroll,
			ClassID:         classRow.ID,
			StartDate:       startDate,
		})
		result.Rows = append(result.Rows, preview(input.Row, action, studentNumber, mergedName, className, changed))
	}
	return result, nil
}

func (s *Service) applyStudents(ctx context.Context, q *db.Queries, rows []studentImportRow, actorID pgtype.UUID) error {
	for _, row := range rows {
		switch row.Action {
		case "create":
			created, err := q.CreateStudent(ctx, row.Create)
			if err != nil {
				return mapWriteError(err)
			}
			if row.Enroll {
				if _, err := q.CreateEnrollment(ctx, db.CreateEnrollmentParams{
					StudentID: created.ID,
					ClassID:   row.ClassID,
					StartDate: row.StartDate,
					CreatedBy: actorID,
				}); err != nil {
					return mapWriteError(err)
				}
			}
		case "update", "unchanged":
			if row.Action == "unchanged" {
				continue
			}
			current, err := q.GetStudentForUpdate(ctx, row.ExistingID)
			if err != nil {
				if database.IsNotFound(err) {
					return ErrPreviewChanged
				}
				return err
			}
			if row.Expected != nil && !sameStudentSnapshot(current, *row.Expected) {
				return ErrPreviewChanged
			}
			if _, err := q.UpdateStudent(ctx, row.Update); err != nil {
				return mapWriteError(err)
			}
			if row.StatusUpdate {
				if row.TargetStatus != "active" {
					if err := s.closeOpenEnrollment(ctx, q, row.ExistingID); err != nil {
						return err
					}
				}
				if _, err := q.UpdateStudentStatus(ctx, db.UpdateStudentStatusParams{
					Status: row.TargetStatus,
					ID:     row.ExistingID,
				}); err != nil {
					return mapWriteError(err)
				}
			} else if row.CloseEnrollment {
				if err := s.closeOpenEnrollment(ctx, q, row.ExistingID); err != nil {
					return err
				}
			}
			if row.Enroll {
				if _, err := q.CreateEnrollment(ctx, db.CreateEnrollmentParams{
					StudentID: row.ExistingID,
					ClassID:   row.ClassID,
					StartDate: row.StartDate,
					CreatedBy: actorID,
				}); err != nil {
					return mapWriteError(err)
				}
			}
		}
	}
	return nil
}

func (s *Service) closeOpenEnrollment(ctx context.Context, q *db.Queries, studentID pgtype.UUID) error {
	open, err := q.GetOpenEnrollmentForStudentForUpdate(ctx, studentID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil
		}
		return err
	}
	endDate := todayDate()
	if endDate.Before(open.StartDate.Time) {
		endDate = open.StartDate.Time
	}
	year, err := q.GetClassYearDates(ctx, open.ClassID)
	if err != nil {
		return err
	}
	if endDate.After(year.EndDate.Time) {
		endDate = year.EndDate.Time
	}
	if endDate.Before(open.StartDate.Time) {
		endDate = open.StartDate.Time
	}
	return q.CloseEnrollment(ctx, db.CloseEnrollmentParams{
		EndDate: pgtype.Date{Time: endDate, Valid: true},
		ID:      open.ID,
	})
}

func sameStudentSnapshot(current, expected db.Student) bool {
	return current.StudentNumber == expected.StudentNumber &&
		current.FullName == expected.FullName &&
		current.AdmissionYear == expected.AdmissionYear &&
		current.Status == expected.Status &&
		sameText(current.NationalStudentNumber, expected.NationalStudentNumber) &&
		sameText(current.Gender, expected.Gender) &&
		dateString(current.BirthDate) == dateString(expected.BirthDate) &&
		sameText(current.GuardianName, expected.GuardianName) &&
		sameText(current.GuardianPhone, expected.GuardianPhone)
}

func sameText(left, right pgtype.Text) bool {
	if left.Valid != right.Valid {
		return false
	}
	if !left.Valid {
		return true
	}
	return left.String == right.String
}

func rowBlank(row []string) bool {
	for _, value := range row {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}
