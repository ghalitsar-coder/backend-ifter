package discipline

import (
	"context"
	"fmt"

	"github.com/xuri/excelize/v2"
)

var disciplineExportHeaders = []string{
	"NIS", "Nama Lengkap", "Kelas", "Tanggal Kejadian", "Kategori",
	"Deskripsi", "Poin", "Petugas Pencatat", "Status Penanganan",
}

func (s *Service) Export(ctx context.Context, actor Actor, filter RecordFilter) ([]byte, error) {
	result, err := s.list(ctx, actor, filter, maxExportRecords, 0)
	if err != nil {
		return nil, err
	}
	file := excelize.NewFile()
	defer func() { _ = file.Close() }()
	const sheet = "Data Kedisiplinan"
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
	for index, header := range disciplineExportHeaders {
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
		switch header {
		case "Nama Lengkap", "Deskripsi":
			width = 30
		case "Kategori", "Petugas Pencatat":
			width = 20
		}
		column, err := excelize.ColumnNumberToName(index + 1)
		if err != nil {
			return nil, err
		}
		if err := file.SetColWidth(sheet, column, column, width); err != nil {
			return nil, err
		}
	}
	lastCell, err := excelize.CoordinatesToCellName(len(disciplineExportHeaders), 1)
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

	var totalPoints int32
	for rowIndex, item := range result.Data {
		row := rowIndex + 2
		values := []any{
			item.StudentNumber, item.StudentName, item.ClassName, item.OccurredDate,
			item.CategoryName, item.Description, item.Points, item.RecordedByName, item.Status,
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
		if item.Status != "cancelled" {
			totalPoints += item.Points
		}
	}

	totalRow := len(result.Data) + 2
	for columnIndex, value := range []any{"Total Poin", "", "", "", "", "", totalPoints, "", ""} {
		cell, err := excelize.CoordinatesToCellName(columnIndex+1, totalRow)
		if err != nil {
			return nil, err
		}
		if err := file.SetCellValue(sheet, cell, value); err != nil {
			return nil, err
		}
	}
	if err := file.SetCellStyle(sheet, fmt.Sprintf("A%d", totalRow), fmt.Sprintf("I%d", totalRow), headerStyle); err != nil {
		return nil, err
	}

	buffer, err := file.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
