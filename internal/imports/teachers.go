package imports

import (
	"bytes"
	"context"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/xuri/excelize/v2"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

var teacherHeaders = map[string][]string{
	"nip":    {"NIP"},
	"name":   {"Nama Lengkap", "Nama"},
	"email":  {"Email"},
	"phone":  {"Telepon", "No Telepon"},
	"status": {"Status"},
}

type teacherInput struct {
	Row            int
	EmployeeNumber string
	FullName       string
	Email          string
	Phone          string
	Status         string
}

type teacherImportRow struct {
	Row            int
	Action         string
	EmployeeNumber string
	FullName       string
	Changed        []string
	ExistingID     pgtype.UUID
	Expected       *db.Teacher
	Create         db.CreateTeacherParams
	Update         db.UpdateTeacherParams
	TargetStatus   string
	StatusUpdate   bool
}

func parseTeacherWorkbook(data []byte, maxRows int) ([]teacherInput, *RowError) {
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
	columns, missing := columnMap(rows[0], teacherHeaders)
	if missing != "" {
		return nil, &RowError{RowNumber: 1, Column: missing, Code: "missing_header", Message: "kolom wajib tidak ditemukan pada header"}
	}

	inputs := make([]teacherInput, 0, len(rows)-1)
	for index := 1; index < len(rows); index++ {
		row := rows[index]
		if rowBlank(row) {
			continue
		}
		inputs = append(inputs, teacherInput{
			Row:            index + 1,
			EmployeeNumber: cell(row, columns, "nip"),
			FullName:       cell(row, columns, "name"),
			Email:          cell(row, columns, "email"),
			Phone:          cell(row, columns, "phone"),
			Status:         cell(row, columns, "status"),
		})
	}
	if len(inputs) == 0 {
		return nil, &RowError{RowNumber: 0, Column: "file", Code: "empty_file", Message: "file tidak memiliki baris data"}
	}
	if len(inputs) > maxRows {
		return nil, &RowError{RowNumber: 0, Column: "file", Code: "too_many_rows", Message: "jumlah baris melebihi batas " + strconv.Itoa(maxRows)}
	}
	return inputs, nil
}

func (s *Service) validateTeachers(ctx context.Context, q *db.Queries, mode Mode, inputs []teacherInput) (Validation, error) {
	result := Validation{}
	seen := make(map[string]int, len(inputs))

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

		employeeNumber, err := requiredText(input.EmployeeNumber, 50)
		if err != nil {
			addError("NIP", "required", "NIP wajib diisi dan maksimal 50 karakter")
		}
		fullName, err := cleanText(input.FullName, 150)
		if err != nil {
			addError("Nama Lengkap", "invalid", "nama maksimal 150 karakter")
		}
		email, err := cleanEmail(input.Email)
		if err != nil {
			addError("Email", "invalid", "email tidak valid")
		}
		phone, err := cleanText(input.Phone, 30)
		if err != nil {
			addError("Telepon", "invalid", "telepon maksimal 30 karakter")
		}
		status, err := normalizeTeacherStatus(input.Status)
		if err != nil {
			addError("Status", "invalid", "status harus active atau inactive")
		}

		if len(rowErrors) > 0 {
			result.InvalidRows++
			result.Errors = append(result.Errors, rowErrors...)
			result.Rows = append(result.Rows, preview(input.Row, "invalid", employeeNumber, fullName, "", nil))
			continue
		}

		if first, exists := seen[employeeNumber]; exists {
			result.DuplicateRows++
			result.Errors = append(result.Errors, RowError{
				RowNumber: input.Row,
				Column:    "NIP",
				Code:      "duplicate_in_file",
				Message:   "NIP duplikat dengan baris " + strconv.Itoa(first),
			})
			result.Rows = append(result.Rows, preview(input.Row, "duplicate", employeeNumber, fullName, "", nil))
			continue
		}
		seen[employeeNumber] = input.Row

		existing, err := q.GetTeacherByEmployeeNumber(ctx, optionalText(employeeNumber))
		if err != nil && !database.IsNotFound(err) {
			return Validation{}, err
		}
		var existingTeacher *db.Teacher
		if err == nil {
			existingTeacher = &existing
		}
		if existingTeacher != nil && mode == ModeCreateOnly {
			result.DuplicateRows++
			result.Errors = append(result.Errors, RowError{
				RowNumber: input.Row,
				Column:    "NIP",
				Code:      "duplicate_nip",
				Message:   "NIP sudah terdaftar; gunakan mode upsert untuk memperbarui data",
			})
			result.Rows = append(result.Rows, preview(input.Row, "duplicate", employeeNumber, fullName, "", nil))
			continue
		}

		targetStatus := status
		if targetStatus == "" {
			if existingTeacher != nil {
				targetStatus = existingTeacher.Status
			} else {
				targetStatus = "active"
			}
		}
		if existingTeacher == nil {
			if fullName == "" {
				result.InvalidRows++
				result.Errors = append(result.Errors, RowError{
					RowNumber: input.Row,
					Column:    "Nama Lengkap",
					Code:      "required",
					Message:   "nama wajib diisi",
				})
				result.Rows = append(result.Rows, preview(input.Row, "invalid", employeeNumber, fullName, "", nil))
				continue
			}
			result.ValidRows++
			result.teacherRows = append(result.teacherRows, teacherImportRow{
				Row:            input.Row,
				Action:         "create",
				EmployeeNumber: employeeNumber,
				FullName:       fullName,
				Create: db.CreateTeacherParams{
					EmployeeNumber: optionalText(employeeNumber),
					FullName:       fullName,
					Email:          optionalText(email),
					Phone:          optionalText(phone),
					Status:         targetStatus,
				},
			})
			result.Rows = append(result.Rows, preview(input.Row, "create", employeeNumber, fullName, "", nil))
			continue
		}

		mergedName := existingTeacher.FullName
		if fullName != "" {
			mergedName = fullName
		}
		mergedEmail := existingTeacher.Email
		if email != "" {
			mergedEmail = optionalText(email)
		}
		mergedPhone := existingTeacher.Phone
		if phone != "" {
			mergedPhone = optionalText(phone)
		}
		changed := make([]string, 0, 4)
		if mergedName != existingTeacher.FullName {
			changed = append(changed, "name")
		}
		if !sameText(mergedEmail, existingTeacher.Email) {
			changed = append(changed, "email")
		}
		if !sameText(mergedPhone, existingTeacher.Phone) {
			changed = append(changed, "phone")
		}
		statusUpdate := status != "" && status != existingTeacher.Status
		if statusUpdate {
			changed = append(changed, "status")
		}
		action := "update"
		if len(changed) == 0 && !statusUpdate {
			action = "unchanged"
		}
		result.ValidRows++
		result.teacherRows = append(result.teacherRows, teacherImportRow{
			Row:            input.Row,
			Action:         action,
			EmployeeNumber: employeeNumber,
			FullName:       mergedName,
			Changed:        changed,
			ExistingID:     existingTeacher.ID,
			Expected:       existingTeacher,
			Update: db.UpdateTeacherParams{
				UserID:         existingTeacher.UserID,
				EmployeeNumber: optionalText(employeeNumber),
				FullName:       mergedName,
				Email:          mergedEmail,
				Phone:          mergedPhone,
				ID:             existingTeacher.ID,
			},
			TargetStatus: targetStatus,
			StatusUpdate: statusUpdate,
		})
		result.Rows = append(result.Rows, preview(input.Row, action, employeeNumber, mergedName, "", changed))
	}
	return result, nil
}

func sameTeacherSnapshot(current, expected db.Teacher) bool {
	return sameUUID(current.UserID, expected.UserID) &&
		sameText(current.EmployeeNumber, expected.EmployeeNumber) &&
		current.FullName == expected.FullName &&
		sameText(current.Email, expected.Email) &&
		sameText(current.Phone, expected.Phone) &&
		current.Status == expected.Status
}

func (s *Service) applyTeachers(ctx context.Context, q *db.Queries, rows []teacherImportRow) error {
	for _, row := range rows {
		switch row.Action {
		case "create":
			if _, err := q.CreateTeacher(ctx, row.Create); err != nil {
				return mapWriteError(err)
			}
		case "update":
			current, err := q.GetTeacherForUpdate(ctx, row.ExistingID)
			if err != nil {
				if database.IsNotFound(err) {
					return ErrPreviewChanged
				}
				return err
			}
			if row.Expected != nil && !sameTeacherSnapshot(current, *row.Expected) {
				return ErrPreviewChanged
			}
			if _, err := q.UpdateTeacher(ctx, row.Update); err != nil {
				return mapWriteError(err)
			}
			if row.StatusUpdate {
				if _, err := q.UpdateTeacherStatus(ctx, db.UpdateTeacherStatusParams{
					Status: row.TargetStatus,
					ID:     row.ExistingID,
				}); err != nil {
					return mapWriteError(err)
				}
			}
		}
	}
	return nil
}
