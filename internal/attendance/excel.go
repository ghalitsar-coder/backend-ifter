package attendance

import (
	"context"
	"fmt"

	"github.com/xuri/excelize/v2"
)

var attendanceExportHeaders = []string{
	"NIS", "Nama Lengkap", "Kelas", "Hari Efektif", "Hadir", "Terlambat",
	"Sakit", "Izin", "Tanpa Keterangan", "Persentase Kehadiran",
}

func (s *Service) Export(ctx context.Context, actor Actor, filter SummaryFilter) ([]byte, error) {
	result, err := s.Summary(ctx, actor, filter)
	if err != nil {
		return nil, err
	}
	file := excelize.NewFile()
	defer func() { _ = file.Close() }()
	const sheet = "Rekap Absensi"
	if err := file.SetSheetName("Sheet1", sheet); err != nil {
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
	percentStyle, err := file.NewStyle(&excelize.Style{
		NumFmt: 10,
		Alignment: &excelize.Alignment{
			Horizontal: "center",
		},
	})
	if err != nil {
		return nil, err
	}
	centerStyle, err := file.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	if err != nil {
		return nil, err
	}
	for index, header := range attendanceExportHeaders {
		cell, err := excelize.CoordinatesToCellName(index+1, 1)
		if err != nil {
			return nil, err
		}
		if err := file.SetCellValue(sheet, cell, header); err != nil {
			return nil, err
		}
		width := float64(len(header) + 4)
		if width < 12 {
			width = 12
		}
		if header == "Nama Lengkap" {
			width = 28
		}
		column, err := excelize.ColumnNumberToName(index + 1)
		if err != nil {
			return nil, err
		}
		if err := file.SetColWidth(sheet, column, column, width); err != nil {
			return nil, err
		}
	}
	lastCell, err := excelize.CoordinatesToCellName(len(attendanceExportHeaders), 1)
	if err != nil {
		return nil, err
	}
	if err := file.SetCellStyle(sheet, "A1", lastCell, headerStyle); err != nil {
		return nil, err
	}
	if err := file.SetRowHeight(sheet, 1, 24); err != nil {
		return nil, err
	}
	if err := file.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
		return nil, err
	}

	for rowIndex, item := range result.Data {
		row := rowIndex + 2
		values := []any{
			item.StudentNumber, item.StudentName, item.ClassName, item.TotalDays,
			item.Present, item.Late, item.Sick, item.Excused, item.Absent,
			item.Percentage / 100,
		}
		for columnIndex, value := range values {
			cell, err := excelize.CoordinatesToCellName(columnIndex+1, row)
			if err != nil {
				return nil, err
			}
			if err := file.SetCellValue(sheet, cell, value); err != nil {
				return nil, err
			}
		}
		if err := file.SetCellStyle(sheet, fmt.Sprintf("D%d", row), fmt.Sprintf("J%d", row), centerStyle); err != nil {
			return nil, err
		}
		percentCell, _ := excelize.CoordinatesToCellName(len(attendanceExportHeaders), row)
		if err := file.SetCellStyle(sheet, percentCell, percentCell, percentStyle); err != nil {
			return nil, err
		}
	}

	totalRow := len(result.Data) + 2
	totals := []any{
		"Total", "", "", result.Totals.TotalDays, result.Totals.Present, result.Totals.Late,
		result.Totals.Sick, result.Totals.Excused, result.Totals.Absent,
		result.Totals.Percentage / 100,
	}
	for columnIndex, value := range totals {
		cell, err := excelize.CoordinatesToCellName(columnIndex+1, totalRow)
		if err != nil {
			return nil, err
		}
		if err := file.SetCellValue(sheet, cell, value); err != nil {
			return nil, err
		}
	}
	if err := file.SetCellStyle(sheet, fmt.Sprintf("A%d", totalRow), fmt.Sprintf("J%d", totalRow), headerStyle); err != nil {
		return nil, err
	}
	percentCell, _ := excelize.CoordinatesToCellName(len(attendanceExportHeaders), totalRow)
	if err := file.SetCellStyle(sheet, percentCell, percentCell, percentStyle); err != nil {
		return nil, err
	}

	buffer, err := file.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

var attendanceReportExportHeaders = []string{
	"Periode Mulai", "Periode Selesai", "Kelas", "Total Tercatat", "Hadir",
	"Terlambat", "Sakit", "Izin", "Tanpa Keterangan", "Persentase Kehadiran",
}

func (s *Service) ExportReport(ctx context.Context, actor Actor, filter ReportFilter) ([]byte, error) {
	result, err := s.Report(ctx, actor, filter)
	if err != nil {
		return nil, err
	}
	file := excelize.NewFile()
	defer func() { _ = file.Close() }()
	const sheet = "Laporan Absensi"
	if err := file.SetSheetName("Sheet1", sheet); err != nil {
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
	percentStyle, err := file.NewStyle(&excelize.Style{
		NumFmt:    10,
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	if err != nil {
		return nil, err
	}
	centerStyle, err := file.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	if err != nil {
		return nil, err
	}
	for index, header := range attendanceReportExportHeaders {
		cell, err := excelize.CoordinatesToCellName(index+1, 1)
		if err != nil {
			return nil, err
		}
		if err := file.SetCellValue(sheet, cell, header); err != nil {
			return nil, err
		}
		width := float64(len(header) + 4)
		if width < 12 {
			width = 12
		}
		if header == "Kelas" {
			width = 24
		}
		column, err := excelize.ColumnNumberToName(index + 1)
		if err != nil {
			return nil, err
		}
		if err := file.SetColWidth(sheet, column, column, width); err != nil {
			return nil, err
		}
	}
	lastCell, err := excelize.CoordinatesToCellName(len(attendanceReportExportHeaders), 1)
	if err != nil {
		return nil, err
	}
	if err := file.SetCellStyle(sheet, "A1", lastCell, headerStyle); err != nil {
		return nil, err
	}
	if err := file.SetRowHeight(sheet, 1, 24); err != nil {
		return nil, err
	}
	if err := file.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
		return nil, err
	}
	for rowIndex, item := range result.Data {
		row := rowIndex + 2
		values := []any{
			item.PeriodStart, item.PeriodEnd, item.ClassName, item.TotalDays,
			item.Present, item.Late, item.Sick, item.Excused, item.Absent,
			item.Percentage / 100,
		}
		for columnIndex, value := range values {
			cell, err := excelize.CoordinatesToCellName(columnIndex+1, row)
			if err != nil {
				return nil, err
			}
			if err := file.SetCellValue(sheet, cell, value); err != nil {
				return nil, err
			}
		}
		if err := file.SetCellStyle(sheet, fmt.Sprintf("D%d", row), fmt.Sprintf("J%d", row), centerStyle); err != nil {
			return nil, err
		}
		percentCell, _ := excelize.CoordinatesToCellName(len(attendanceReportExportHeaders), row)
		if err := file.SetCellStyle(sheet, percentCell, percentCell, percentStyle); err != nil {
			return nil, err
		}
	}
	totalRow := len(result.Data) + 2
	totals := []any{
		"Total", "", "", result.Totals.TotalDays, result.Totals.Present, result.Totals.Late,
		result.Totals.Sick, result.Totals.Excused, result.Totals.Absent,
		result.Totals.Percentage / 100,
	}
	for columnIndex, value := range totals {
		cell, err := excelize.CoordinatesToCellName(columnIndex+1, totalRow)
		if err != nil {
			return nil, err
		}
		if err := file.SetCellValue(sheet, cell, value); err != nil {
			return nil, err
		}
	}
	if err := file.SetCellStyle(sheet, fmt.Sprintf("A%d", totalRow), fmt.Sprintf("J%d", totalRow), headerStyle); err != nil {
		return nil, err
	}
	percentCell, _ := excelize.CoordinatesToCellName(len(attendanceReportExportHeaders), totalRow)
	if err := file.SetCellStyle(sheet, percentCell, percentCell, percentStyle); err != nil {
		return nil, err
	}
	buffer, err := file.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
