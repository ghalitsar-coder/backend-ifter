package attendance

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

func (s *Service) Report(ctx context.Context, actor Actor, filter ReportFilter) (ReportResult, error) {
	from, to, err := s.reportRange(ctx, filter.From, filter.To, filter.TermID)
	if err != nil {
		return ReportResult{}, err
	}
	groupBy := strings.ToLower(strings.TrimSpace(filter.GroupBy))
	if groupBy == "" {
		groupBy = "day"
	}
	if groupBy != "day" && groupBy != "week" && groupBy != "month" {
		return ReportResult{}, ErrInvalidInput
	}
	studentID, err := optionalUUID(filter.StudentID)
	if err != nil {
		return ReportResult{}, ErrInvalidInput
	}
	classID, err := optionalUUID(filter.ClassID)
	if err != nil {
		return ReportResult{}, ErrInvalidInput
	}
	scope, err := resolveScope(ctx, s.queries, actor)
	if err != nil {
		return ReportResult{}, err
	}
	rows, err := s.queries.ListAttendanceReport(ctx, db.ListAttendanceReportParams{
		Period:    groupBy,
		FromDate:  from,
		ToDate:    to,
		StudentID: studentID,
		ClassID:   classID,
		AllowAll:  scope.allowAll,
		IsTeacher: scope.isTeacher,
		TeacherID: scope.teacherID,
		IsDuty:    scope.isDuty,
	})
	if err != nil {
		return ReportResult{}, fmt.Errorf("list attendance report: %w", err)
	}
	items := make([]ReportRow, 0, len(rows))
	totals := ReportTotal{}
	for _, row := range rows {
		item := ReportRow{
			PeriodStart: dateString(row.PeriodStart),
			PeriodEnd:   reportPeriodEnd(row.PeriodStart, groupBy),
			ClassID:     uuidString(row.ClassID),
			ClassName:   row.ClassName,
			TotalDays:   row.TotalDays,
			Present:     row.Present,
			Late:        row.Late,
			Sick:        row.Sick,
			Excused:     row.Excused,
			Absent:      row.Absent,
		}
		item.Percentage = percentage(item.Present, item.Late, item.TotalDays)
		items = append(items, item)
		totals.TotalDays += item.TotalDays
		totals.Present += item.Present
		totals.Late += item.Late
		totals.Sick += item.Sick
		totals.Excused += item.Excused
		totals.Absent += item.Absent
	}
	totals.Percentage = percentage(totals.Present, totals.Late, totals.TotalDays)
	return ReportResult{
		From:    dateString(from),
		To:      dateString(to),
		GroupBy: groupBy,
		Data:    items,
		Totals:  totals,
	}, nil
}

func (s *Service) reportRange(ctx context.Context, fromValue, toValue, termID string) (pgtype.Date, pgtype.Date, error) {
	if strings.TrimSpace(termID) == "" {
		return validateDateRange(fromValue, toValue)
	}
	id, err := parseUUID(termID)
	if err != nil {
		return pgtype.Date{}, pgtype.Date{}, ErrInvalidInput
	}
	term, err := s.queries.GetTerm(ctx, id)
	if err != nil {
		if database.IsNotFound(err) {
			return pgtype.Date{}, pgtype.Date{}, ErrNotFound
		}
		return pgtype.Date{}, pgtype.Date{}, fmt.Errorf("get term for attendance report: %w", err)
	}
	return validateDateRange(dateString(term.StartDate), dateString(term.EndDate))
}

func reportPeriodEnd(start pgtype.Date, groupBy string) string {
	if !start.Valid {
		return ""
	}
	end := start.Time
	switch groupBy {
	case "week":
		end = end.AddDate(0, 0, 6)
	case "month":
		end = time.Date(end.Year(), end.Month()+1, 0, 0, 0, 0, 0, time.UTC)
	}
	return end.Format("2006-01-02")
}

func percentage(present, late, total int32) float64 {
	if total == 0 {
		return 0
	}
	value := float64(present+late) / float64(total) * 100
	return float64(int(value*100+0.5)) / 100
}
