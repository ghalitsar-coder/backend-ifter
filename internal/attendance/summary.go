package attendance

import (
	"context"
	"fmt"
	"strings"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
)

func (s *Service) History(ctx context.Context, actor Actor, filter HistoryFilter) (HistoryResult, error) {
	from, to, err := validateDateRange(filter.From, filter.To)
	if err != nil {
		return HistoryResult{}, err
	}
	studentID, err := optionalUUID(filter.StudentID)
	if err != nil {
		return HistoryResult{}, ErrInvalidInput
	}
	classID, err := optionalUUID(filter.ClassID)
	if err != nil {
		return HistoryResult{}, ErrInvalidInput
	}
	status := strings.TrimSpace(filter.Status)
	if status != "" && !validStatus(status) {
		return HistoryResult{}, ErrInvalidInput
	}
	scope, err := resolveScope(ctx, s.queries, actor)
	if err != nil {
		return HistoryResult{}, err
	}
	limit, offset := normalizePage(filter.Limit, filter.Offset)
	params := db.ListAttendanceHistoryParams{
		FromDate:   from,
		ToDate:     to,
		StudentID:  studentID,
		ClassID:    classID,
		Status:     optionalText(status),
		AllowAll:   scope.allowAll,
		IsTeacher:  scope.isTeacher,
		TeacherID:  scope.teacherID,
		IsDuty:     scope.isDuty,
		PageOffset: offset,
		PageLimit:  limit,
	}
	rows, err := s.queries.ListAttendanceHistory(ctx, params)
	if err != nil {
		return HistoryResult{}, fmt.Errorf("list attendance history: %w", err)
	}
	total, err := s.queries.CountAttendanceHistory(ctx, db.CountAttendanceHistoryParams{
		FromDate:  from,
		ToDate:    to,
		StudentID: studentID,
		ClassID:   classID,
		Status:    optionalText(status),
		AllowAll:  scope.allowAll,
		IsTeacher: scope.isTeacher,
		TeacherID: scope.teacherID,
		IsDuty:    scope.isDuty,
	})
	if err != nil {
		return HistoryResult{}, fmt.Errorf("count attendance history: %w", err)
	}
	items := make([]HistoryRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, historyFromRow(row))
	}
	return HistoryResult{Data: items, Total: total, Limit: limit, Offset: offset}, nil
}

func (s *Service) Summary(ctx context.Context, actor Actor, filter SummaryFilter) (SummaryResult, error) {
	from, to, err := validateDateRange(filter.From, filter.To)
	if err != nil {
		return SummaryResult{}, err
	}
	studentID, err := optionalUUID(filter.StudentID)
	if err != nil {
		return SummaryResult{}, ErrInvalidInput
	}
	classID, err := optionalUUID(filter.ClassID)
	if err != nil {
		return SummaryResult{}, ErrInvalidInput
	}
	scope, err := resolveScope(ctx, s.queries, actor)
	if err != nil {
		return SummaryResult{}, err
	}
	rows, err := s.queries.ListAttendanceSummary(ctx, db.ListAttendanceSummaryParams{
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
		return SummaryResult{}, fmt.Errorf("list attendance summary: %w", err)
	}
	items := make([]StudentSummary, 0, len(rows))
	totals := ReportTotals{}
	for _, row := range rows {
		item := summaryFromRow(row)
		items = append(items, item)
		totals.Students++
		totals.TotalDays += item.TotalDays
		totals.Present += item.Present
		totals.Late += item.Late
		totals.Sick += item.Sick
		totals.Excused += item.Excused
		totals.Absent += item.Absent
	}
	if totals.TotalDays > 0 {
		totals.Percentage = float64(totals.Present+totals.Late) / float64(totals.TotalDays) * 100
		totals.Percentage = float64(int(totals.Percentage*100+0.5)) / 100
	}
	return SummaryResult{
		From:   dateString(from),
		To:     dateString(to),
		Data:   items,
		Totals: totals,
	}, nil
}
