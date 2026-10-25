package studentaccounts

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

// MyProfile returns the student row the login is linked to. It exists because
// GET /auth/me answers from the auth package's user payload, which this
// package must not change; a student portal reads its own id here instead.
func (s *Service) MyProfile(ctx context.Context, actor Actor) (ProfileEnvelope, error) {
	student, err := s.myStudent(ctx, actor)
	if err != nil {
		return ProfileEnvelope{}, err
	}
	detail, err := s.queries.GetStudent(ctx, student.ID)
	if err != nil {
		if database.IsNotFound(err) {
			return ProfileEnvelope{}, ErrAccountNotLinked
		}
		return ProfileEnvelope{}, fmt.Errorf("get student profile: %w", err)
	}
	return ProfileEnvelope{Profile: Profile{
		StudentID:     uuidString(detail.ID),
		StudentNumber: detail.StudentNumber,
		FullName:      detail.FullName,
		ClassID:       uuidPtr(detail.CurrentClassID),
		ClassName:     nullableString(detail.CurrentClassName),
	}}, nil
}

// MyAttendance is GET /my/attendance: the actor's own attendance records over
// the shared attendance-history query, with the student id forced from the
// token (a student never supplies one).
func (s *Service) MyAttendance(ctx context.Context, actor Actor, filter DateRangeFilter) (MyAttendanceList, error) {
	student, err := s.myStudent(ctx, actor)
	if err != nil {
		return MyAttendanceList{}, err
	}
	from, to, err := parseDateRange(filter.From, filter.To)
	if err != nil {
		return MyAttendanceList{}, err
	}
	limit, offset := normalizePage(filter.Limit, filter.Offset)
	rows, err := s.queries.ListAttendanceHistory(ctx, s.myAttendanceParams(student.ID, from, to, limit, offset))
	if err != nil {
		return MyAttendanceList{}, fmt.Errorf("list my attendance: %w", err)
	}
	total, err := s.queries.CountAttendanceHistory(ctx, s.myAttendanceCountParams(student.ID, from, to))
	if err != nil {
		return MyAttendanceList{}, fmt.Errorf("count my attendance: %w", err)
	}
	items := make([]MyAttendanceItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, MyAttendanceItem{
			Date:        dateString(row.AttendanceDate),
			Status:      row.Status,
			CheckInTime: timestampPtr(row.CheckInAt),
			ClassID:     uuidString(row.ClassID),
			ClassName:   row.ClassName,
		})
	}
	return MyAttendanceList{Data: items, Total: total, Limit: limit, Offset: offset}, nil
}

// MyAttendanceSummary is GET /my/attendance-summary: the same aggregation the
// staff GET /attendance/summary performs (the shared ListAttendanceSummary
// query), restricted to the actor's own student.
func (s *Service) MyAttendanceSummary(ctx context.Context, actor Actor, filter DateRangeFilter) (MyAttendanceSummary, error) {
	student, err := s.myStudent(ctx, actor)
	if err != nil {
		return MyAttendanceSummary{}, err
	}
	from, to, err := parseDateRange(filter.From, filter.To)
	if err != nil {
		return MyAttendanceSummary{}, err
	}
	rows, err := s.queries.ListAttendanceSummary(ctx, db.ListAttendanceSummaryParams{
		FromDate:  from,
		ToDate:    to,
		StudentID: student.ID,
		AllowAll:  true,
	})
	if err != nil {
		return MyAttendanceSummary{}, fmt.Errorf("list my attendance summary: %w", err)
	}
	items := make([]MyAttendanceDay, 0, len(rows))
	totals := MyAttendanceTotals{}
	for _, row := range rows {
		item := MyAttendanceDay{
			StudentID:     uuidString(row.StudentID),
			StudentNumber: row.StudentNumber,
			StudentName:   row.FullName,
			ClassID:       uuidString(row.ClassID),
			ClassName:     row.ClassName,
			TotalDays:     row.TotalDays,
			Present:       row.PresentDays,
			Late:          row.LateDays,
			Sick:          row.SickDays,
			Excused:       row.ExcusedDays,
			Absent:        row.AbsentDays,
			Percentage:    attendancePercentage(row.PresentDays+row.LateDays, row.TotalDays),
		}
		items = append(items, item)
		totals.Students++
		totals.TotalDays += item.TotalDays
		totals.Present += item.Present
		totals.Late += item.Late
		totals.Sick += item.Sick
		totals.Excused += item.Excused
		totals.Absent += item.Absent
	}
	totals.Percentage = attendancePercentage(totals.Present+totals.Late, totals.TotalDays)
	return MyAttendanceSummary{
		From:   dateString(from),
		To:     dateString(to),
		Data:   items,
		Totals: totals,
	}, nil
}

// MyDiscipline is GET /my/discipline: the actor's own discipline records over
// the shared discipline-records query with the student id forced.
func (s *Service) MyDiscipline(ctx context.Context, actor Actor, filter DateRangeFilter) (MyDisciplineList, error) {
	student, err := s.myStudent(ctx, actor)
	if err != nil {
		return MyDisciplineList{}, err
	}
	from, to, err := parseDateRange(filter.From, filter.To)
	if err != nil {
		return MyDisciplineList{}, err
	}
	limit, offset := normalizePage(filter.Limit, filter.Offset)
	rows, err := s.queries.ListDisciplineRecords(ctx, s.myDisciplineParams(student.ID, from, to, limit, offset))
	if err != nil {
		return MyDisciplineList{}, fmt.Errorf("list my discipline records: %w", err)
	}
	total, err := s.queries.CountDisciplineRecords(ctx, s.myDisciplineCountParams(student.ID, from, to))
	if err != nil {
		return MyDisciplineList{}, fmt.Errorf("count my discipline records: %w", err)
	}
	items := make([]MyDisciplineItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, MyDisciplineItem{
			ID:           uuidString(row.ID),
			OccurredAt:   timestamp(row.OccurredAt),
			CategoryName: row.CategoryName,
			Description:  row.Description,
			Points:       row.Points,
			Status:       row.Status,
		})
	}
	return MyDisciplineList{Data: items, Total: total, Limit: limit, Offset: offset}, nil
}

// MyDisciplineSummary is GET /my/discipline-summary: the aggregate the staff
// GET /discipline/students/:id builds (points total, per-category breakdown and
// the sanction tier the points reach), scoped to the actor's own student.
func (s *Service) MyDisciplineSummary(ctx context.Context, actor Actor, filter DateRangeFilter) (MyDisciplineSummary, error) {
	student, err := s.myStudent(ctx, actor)
	if err != nil {
		return MyDisciplineSummary{}, err
	}
	from, to, err := parseDateRange(filter.From, filter.To)
	if err != nil {
		return MyDisciplineSummary{}, err
	}
	rows, err := s.queries.ListDisciplineRecords(ctx, s.myDisciplineParams(student.ID, from, to, maxSummaryRecords, 0))
	if err != nil {
		return MyDisciplineSummary{}, fmt.Errorf("list my discipline summary: %w", err)
	}
	summary := MyDisciplineSummary{
		StudentID:     uuidString(student.ID),
		StudentNumber: student.StudentNumber,
		StudentName:   student.FullName,
		Categories:    make([]MyDisciplineCategory, 0),
	}
	index := make(map[string]int)
	for _, row := range rows {
		// Cancelled records carry no sanction weight (same rule as the staff
		// summary in internal/discipline).
		if row.Status == "cancelled" {
			continue
		}
		summary.TotalRecords++
		summary.TotalPoints += row.Points
		categoryID := uuidString(row.CategoryID)
		position, exists := index[categoryID]
		if !exists {
			position = len(summary.Categories)
			index[categoryID] = position
			summary.Categories = append(summary.Categories, MyDisciplineCategory{
				CategoryID:   categoryID,
				CategoryName: row.CategoryName,
			})
		}
		summary.Categories[position].Count++
		summary.Categories[position].Points += row.Points
	}
	// current_level: the active sanction rule covering the accumulated points
	// (sanction-rules package); null when no rule matches.
	if level, err := s.queries.MatchSanctionRuleForPoints(ctx, summary.TotalPoints); err == nil {
		summary.CurrentLevel = &MyDisciplineLevel{Level: textPtr(level.Level), Action: level.Action}
	} else if !database.IsNotFound(err) {
		return MyDisciplineSummary{}, fmt.Errorf("match sanction rule for my points: %w", err)
	}
	return summary, nil
}

// myStudent resolves the actor's own student row through students.user_id. A
// student-role login without a linked active student gets 403
// (ErrAccountNotLinked), exactly like the /my/dispensations surface.
func (s *Service) myStudent(ctx context.Context, actor Actor) (db.GetActiveStudentByUserIDRow, error) {
	if err := requireStudentActor(actor); err != nil {
		return db.GetActiveStudentByUserIDRow{}, err
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return db.GetActiveStudentByUserIDRow{}, ErrUnauthorized
	}
	student, err := s.queries.GetActiveStudentByUserID(ctx, actorID)
	if err != nil {
		if database.IsNotFound(err) {
			return db.GetActiveStudentByUserIDRow{}, ErrAccountNotLinked
		}
		return db.GetActiveStudentByUserIDRow{}, fmt.Errorf("find student for actor: %w", err)
	}
	if !student.ID.Valid {
		return db.GetActiveStudentByUserIDRow{}, ErrAccountNotLinked
	}
	return student, nil
}

// myAttendanceParams builds the shared history query parameters for one
// student. AllowAll is set only because StudentID is already pinned to the
// actor's own row: the scope is the identity of the caller, not a privilege.
func (s *Service) myAttendanceParams(studentID pgtype.UUID, from, to pgtype.Date, limit, offset int32) db.ListAttendanceHistoryParams {
	return db.ListAttendanceHistoryParams{
		FromDate:   from,
		ToDate:     to,
		StudentID:  studentID,
		AllowAll:   true,
		PageOffset: offset,
		PageLimit:  limit,
	}
}

// myAttendanceCountParams mirrors myAttendanceParams for the count query.
func (s *Service) myAttendanceCountParams(studentID pgtype.UUID, from, to pgtype.Date) db.CountAttendanceHistoryParams {
	return db.CountAttendanceHistoryParams{
		FromDate:  from,
		ToDate:    to,
		StudentID: studentID,
		AllowAll:  true,
	}
}

// myDisciplineParams builds the shared discipline-records query parameters for
// one student.
func (s *Service) myDisciplineParams(studentID pgtype.UUID, from, to pgtype.Date, limit, offset int32) db.ListDisciplineRecordsParams {
	return db.ListDisciplineRecordsParams{
		StudentID:  studentID,
		FromDate:   from,
		ToDate:     to,
		AllowAll:   true,
		PageOffset: offset,
		PageLimit:  limit,
	}
}

// myDisciplineCountParams mirrors myDisciplineParams for the count query.
func (s *Service) myDisciplineCountParams(studentID pgtype.UUID, from, to pgtype.Date) db.CountDisciplineRecordsParams {
	return db.CountDisciplineRecordsParams{
		StudentID: studentID,
		FromDate:  from,
		ToDate:    to,
		AllowAll:  true,
	}
}

func nullableString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
