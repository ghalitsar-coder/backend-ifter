package discipline

import (
	"context"
	"fmt"
	"strings"

	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

func (s *Service) Report(ctx context.Context, actor Actor, filter ReportFilter) (Report, error) {
	if strings.TrimSpace(filter.TermID) != "" {
		termID, err := parseUUID(filter.TermID)
		if err != nil {
			return Report{}, ErrInvalidInput
		}
		term, err := s.queries.GetTerm(ctx, termID)
		if err != nil {
			if database.IsNotFound(err) {
				return Report{}, ErrNotFound
			}
			return Report{}, fmt.Errorf("get term for discipline report: %w", err)
		}
		filter.From = dateString(term.StartDate)
		filter.To = dateString(term.EndDate)
	}
	result, err := s.list(ctx, actor, RecordFilter{
		From:       filter.From,
		To:         filter.To,
		StudentID:  filter.StudentID,
		ClassID:    filter.ClassID,
		CategoryID: filter.CategoryID,
		RecordedBy: filter.RecordedBy,
		Status:     filter.Status,
	}, maxExportRecords, 0)
	if err != nil {
		return Report{}, err
	}
	report := Report{
		From:         strings.TrimSpace(filter.From),
		To:           strings.TrimSpace(filter.To),
		TotalRecords: int(result.Total),
		ByCategory:   make([]CategorySummary, 0),
		ByStatus:     make([]StatusSummary, 0),
		Data:         result.Data,
	}
	categoryIndex := make(map[string]int)
	statusCounts := map[string]int{
		"new": 0, "under_review": 0, "in_followup": 0, "completed": 0, "cancelled": 0,
	}
	for _, record := range result.Data {
		statusCounts[record.Status]++
		if record.Status == "cancelled" {
			report.CancelledRecords++
			continue
		}
		report.TotalPoints += record.Points
		position, exists := categoryIndex[record.CategoryID]
		if !exists {
			position = len(report.ByCategory)
			categoryIndex[record.CategoryID] = position
			report.ByCategory = append(report.ByCategory, CategorySummary{
				CategoryID:   record.CategoryID,
				CategoryName: record.CategoryName,
			})
		}
		report.ByCategory[position].Count++
		report.ByCategory[position].Points += record.Points
	}
	for _, status := range []string{"new", "under_review", "in_followup", "completed", "cancelled"} {
		report.ByStatus = append(report.ByStatus, StatusSummary{Status: status, Count: statusCounts[status]})
	}
	return report, nil
}
