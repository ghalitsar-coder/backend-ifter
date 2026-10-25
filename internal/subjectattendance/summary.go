package subjectattendance

import (
	"context"
	"fmt"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
)

// Summary is GET /reports/subject-attendance: per-assignment recap over a
// mandatory date range. A plain teacher only sees their own assignments.
func (s *Service) Summary(ctx context.Context, actor Actor, filter SummaryFilter) (Summary, error) {
	scope, err := s.resolveScope(ctx, actor, false)
	if err != nil {
		return Summary{}, err
	}
	from, to, err := parseDateRange(filter.From, filter.To)
	if err != nil {
		return Summary{}, err
	}
	params := db.SummarizeAttendanceSessionsParams{FromDate: from, ToDate: to}
	for name, raw := range map[string]string{
		"class_id":   filter.ClassID,
		"subject_id": filter.SubjectID,
		"teacher_id": filter.TeacherID,
	} {
		if raw == "" {
			continue
		}
		id, err := parseUUID(raw)
		if err != nil {
			return Summary{}, ErrInvalidInput
		}
		switch name {
		case "class_id":
			params.ClassID = id
		case "subject_id":
			params.SubjectID = id
		case "teacher_id":
			params.TeacherID = id
		}
	}
	if scope.restrict {
		params.TeacherID = scope.teacherID
	}
	rows, err := s.queries.SummarizeAttendanceSessions(ctx, params)
	if err != nil {
		return Summary{}, fmt.Errorf("summarize attendance sessions: %w", err)
	}

	result := Summary{From: dateString(from), To: dateString(to), Data: make([]SummaryRow, 0, len(rows))}
	var totals SummaryTotals
	for _, row := range rows {
		item := SummaryRow{
			TeachingAssignmentID: uuidString(row.TeachingAssignmentID),
			ClassID:              uuidString(row.ClassID),
			ClassName:            row.ClassName,
			SubjectID:            uuidString(row.SubjectID),
			SubjectCode:          row.SubjectCode,
			SubjectName:          row.SubjectName,
			TeacherID:            uuidString(row.TeacherID),
			TeacherName:          row.TeacherName,
			Sessions:             row.Sessions,
			Records:              row.Records,
			Present:              row.Present,
			Late:                 row.Late,
			Sick:                 row.Sick,
			Excused:              row.Excused,
			Absent:               row.Absent,
		}
		item.Percentage = attendancePercentage(row.Present+row.Late, row.Records)
		result.Data = append(result.Data, item)

		totals.Sessions += row.Sessions
		totals.Records += row.Records
		totals.Present += row.Present
		totals.Late += row.Late
		totals.Sick += row.Sick
		totals.Excused += row.Excused
		totals.Absent += row.Absent
	}
	totals.Percentage = attendancePercentage(totals.Present+totals.Late, totals.Records)
	result.Totals = totals
	return result, nil
}
