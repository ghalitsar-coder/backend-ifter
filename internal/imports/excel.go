package imports

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/xuri/excelize/v2"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
)

var (
	studentTemplateHeaders = []string{"NIS", "NISN", "Nama Lengkap", "Jenis Kelamin", "Tanggal Lahir", "Tahun Masuk", "Status", "Nama Wali", "Telepon Wali", "Tahun Ajaran", "Kelas", "Tanggal Mulai Kelas"}
	teacherTemplateHeaders = []string{"NIP", "Nama Lengkap", "Email", "Telepon", "Status", "Jenis Kepegawaian"}
	studentExportHeaders   = studentTemplateHeaders
	teacherExportHeaders   = []string{"NIP", "Nama Lengkap", "Email", "Telepon", "Status", "Jenis Kepegawaian"}
)

func (s *Service) Template(entity Entity) ([]byte, error) {
	switch entity {
	case EntityStudents:
		return buildTemplate("Data Siswa", studentTemplateHeaders, []string{
			"Kolom NIS, Nama Lengkap, dan Tahun Masuk wajib diisi.",
			"Jenis Kelamin: L atau P. Tanggal memakai format YYYY-MM-DD.",
			"Status: active, graduated, transferred, inactive. Kosong berarti active.",
			"Kelas diisi bersama Tahun Ajaran. Tahun Ajaran kosong berarti tahun ajaran aktif.",
			"Tanggal Mulai Kelas harus berada di dalam rentang tahun ajaran. Kosong berarti hari ini.",
			"Format kolom NIS dan NISN sebagai teks agar angka nol di depan tidak hilang.",
			"Sheet Contoh hanya ilustrasi format; isi data pada sheet Data Siswa.",
		}, []string{
			"10001", "1234567890", "Contoh Siswa", "L", "2010-01-01", "2026",
			"active", "Contoh Wali", "081234567890", "2026/2027", "10A", "2026-07-01",
		})
	case EntityTeachers:
		return buildTemplate("Data Guru", teacherTemplateHeaders, []string{
			"Kolom NIP dan Nama Lengkap wajib diisi.",
			"Email harus valid jika diisi. Status: active atau inactive.",
			"Jenis Kepegawaian opsional: Permanent, Honorer, atau Staf TU. Nilai tidak dikenal = baris tetap diimpor tanpa jenis kepegawaian (peringatan).",
			"Format kolom NIP sebagai teks agar angka nol di depan tidak hilang.",
			"Import tidak membuat akun atau password login.",
			"Sheet Contoh hanya ilustrasi format; isi data pada sheet Data Guru.",
		}, []string{
			"T0001", "Contoh Guru", "guru@example.com", "081234567890", "active", "Permanent",
		})
	default:
		return nil, ErrInvalidInput
	}
}

func buildTemplate(sheetName string, headers, notes, example []string) ([]byte, error) {
	file := excelize.NewFile()
	defer func() { _ = file.Close() }()
	if err := file.SetSheetName("Sheet1", sheetName); err != nil {
		return nil, err
	}
	if err := writeHeader(file, sheetName, headers); err != nil {
		return nil, err
	}
	headerStyle, err := file.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"2F5597"}},
		Alignment: &excelize.Alignment{
			Horizontal: "center",
			Vertical:   "center",
		},
	})
	if err != nil {
		return nil, err
	}
	lastCell, err := excelize.CoordinatesToCellName(len(headers), 1)
	if err != nil {
		return nil, err
	}
	if err := file.SetCellStyle(sheetName, "A1", lastCell, headerStyle); err != nil {
		return nil, err
	}
	if err := file.SetRowHeight(sheetName, 1, 24); err != nil {
		return nil, err
	}
	if err := file.SetPanes(sheetName, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
		return nil, err
	}
	if err := setTemplateWidths(file, sheetName, headers); err != nil {
		return nil, err
	}

	exampleSheet := "Contoh"
	if _, err := file.NewSheet(exampleSheet); err != nil {
		return nil, err
	}
	if err := writeHeader(file, exampleSheet, headers); err != nil {
		return nil, err
	}
	exampleLastCell, err := excelize.CoordinatesToCellName(len(headers), 1)
	if err != nil {
		return nil, err
	}
	if err := file.SetCellStyle(exampleSheet, "A1", exampleLastCell, headerStyle); err != nil {
		return nil, err
	}
	for index, value := range example {
		cell, err := excelize.CoordinatesToCellName(index+1, 2)
		if err != nil {
			return nil, err
		}
		if err := file.SetCellValue(exampleSheet, cell, value); err != nil {
			return nil, err
		}
	}
	if err := setTemplateWidths(file, exampleSheet, headers); err != nil {
		return nil, err
	}

	guide := "Petunjuk"
	if _, err := file.NewSheet(guide); err != nil {
		return nil, err
	}
	if err := file.SetColWidth(guide, "A", "A", 100); err != nil {
		return nil, err
	}
	for index, note := range notes {
		cell, err := excelize.CoordinatesToCellName(1, index+1)
		if err != nil {
			return nil, err
		}
		if err := file.SetCellValue(guide, cell, note); err != nil {
			return nil, err
		}
	}
	buffer, err := file.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func setTemplateWidths(file *excelize.File, sheet string, headers []string) error {
	for index, header := range headers {
		column, err := excelize.ColumnNumberToName(index + 1)
		if err != nil {
			return err
		}
		width := float64(len(header) + 4)
		if width < 12 {
			width = 12
		}
		if header == "Nama Lengkap" || header == "Nama Wali" {
			width = 28
		}
		if err := file.SetColWidth(sheet, column, column, width); err != nil {
			return err
		}
	}
	return nil
}

func writeHeader(file *excelize.File, sheet string, headers []string) error {
	for index, header := range headers {
		cell, err := excelize.CoordinatesToCellName(index+1, 1)
		if err != nil {
			return err
		}
		if err := file.SetCellValue(sheet, cell, header); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ExportStudents(ctx context.Context, filter ExportFilter) ([]byte, error) {
	params := db.ListStudentsForExportParams{Status: pgtype.Text{}, ClassID: pgtype.UUID{}, Search: pgtype.Text{}}
	if filter.Status != "" {
		switch filter.Status {
		case "active", "graduated", "transferred", "inactive":
			params.Status = optionalText(filter.Status)
		default:
			return nil, ErrInvalidInput
		}
	}
	if filter.ClassID != "" {
		classID, err := parseUUID(filter.ClassID)
		if err != nil {
			return nil, ErrInvalidInput
		}
		params.ClassID = classID
	}
	search, err := cleanText(filter.Search, 150)
	if err != nil {
		return nil, err
	}
	params.Search = optionalText(search)

	rows, err := s.queries.ListStudentsForExport(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list students for export: %w", err)
	}
	data := make([][]string, 0, len(rows))
	for _, row := range rows {
		data = append(data, []string{
			row.StudentNumber,
			row.NationalStudentNumber.String,
			row.FullName,
			row.Gender.String,
			dateString(row.BirthDate),
			fmt.Sprintf("%d", row.AdmissionYear),
			row.Status,
			row.GuardianName.String,
			row.GuardianPhone.String,
			row.CurrentAcademicYearName,
			row.CurrentClassName,
			dateString(row.CurrentEnrollmentStart),
		})
	}
	return buildExport("Data Siswa", studentExportHeaders, data)
}

func (s *Service) ExportTeachers(ctx context.Context, filter ExportFilter) ([]byte, error) {
	params := db.ListTeachersForExportParams{Status: pgtype.Text{}, Search: pgtype.Text{}}
	if filter.Status != "" {
		switch filter.Status {
		case "active", "inactive":
			params.Status = optionalText(filter.Status)
		default:
			return nil, ErrInvalidInput
		}
	}
	search, err := cleanText(filter.Search, 150)
	if err != nil {
		return nil, err
	}
	params.Search = optionalText(search)

	rows, err := s.queries.ListTeachersForExport(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list teachers for export: %w", err)
	}
	data := make([][]string, 0, len(rows))
	for _, row := range rows {
		data = append(data, []string{
			row.EmployeeNumber.String,
			row.FullName,
			row.Email.String,
			row.Phone.String,
			row.Status,
			row.EmploymentTypeName.String,
		})
	}
	return buildExport("Data Guru", teacherExportHeaders, data)
}

func buildExport(sheetName string, headers []string, rows [][]string) ([]byte, error) {
	file := excelize.NewFile()
	defer func() { _ = file.Close() }()
	if err := file.SetSheetName("Sheet1", sheetName); err != nil {
		return nil, err
	}
	if err := writeHeader(file, sheetName, headers); err != nil {
		return nil, err
	}
	headerStyle, err := file.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"2F5597"}},
	})
	if err != nil {
		return nil, err
	}
	lastCell, err := excelize.CoordinatesToCellName(len(headers), 1)
	if err != nil {
		return nil, err
	}
	if err := file.SetCellStyle(sheetName, "A1", lastCell, headerStyle); err != nil {
		return nil, err
	}
	if err := file.SetPanes(sheetName, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
		return nil, err
	}
	for rowIndex, row := range rows {
		for columnIndex, value := range row {
			if strings.TrimSpace(value) == "" {
				continue
			}
			cell, err := excelize.CoordinatesToCellName(columnIndex+1, rowIndex+2)
			if err != nil {
				return nil, err
			}
			if err := file.SetCellValue(sheetName, cell, value); err != nil {
				return nil, err
			}
		}
	}
	for index, header := range headers {
		column, err := excelize.ColumnNumberToName(index + 1)
		if err != nil {
			return nil, err
		}
		width := float64(len(header) + 4)
		if width < 12 {
			width = 12
		}
		if header == "Nama Lengkap" || header == "Nama Wali" {
			width = 28
		}
		if err := file.SetColWidth(sheetName, column, column, width); err != nil {
			return nil, err
		}
	}
	buffer, err := file.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
