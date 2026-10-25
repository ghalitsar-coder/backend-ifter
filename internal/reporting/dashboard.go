package reporting

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
)

func (s *Service) Dashboard(ctx context.Context, actor Actor, filter DashboardFilter) (Dashboard, error) {
	scope, err := resolveScope(ctx, s.queries, actor)
	if err != nil {
		return Dashboard{}, err
	}
	date, loc, err := s.dashboardDate(ctx, filter.Date)
	if err != nil {
		return Dashboard{}, err
	}
	classID, err := optionalUUID(filter.ClassID)
	if err != nil {
		return Dashboard{}, ErrInvalidInput
	}
	if err := canReadClass(ctx, s.queries, scope, classID, date); err != nil {
		return Dashboard{}, err
	}

	attendance, err := s.queries.DashboardAttendanceSummary(ctx, db.DashboardAttendanceSummaryParams{
		AttendanceDate: date,
		ClassID:        classID,
		AllowAll:       scope.allowAll,
		IsTeacher:      scope.isTeacher,
		TeacherID:      scope.teacherID,
		IsDuty:         scope.isDuty,
	})
	if err != nil {
		return Dashboard{}, fmt.Errorf("dashboard attendance summary: %w", err)
	}
	monthStart := time.Date(date.Time.Year(), date.Time.Month(), 1, 0, 0, 0, 0, loc)
	discipline, err := s.queries.DashboardDisciplineSummary(ctx, db.DashboardDisciplineSummaryParams{
		AttendanceDate: date,
		MonthStartAt:   timestampAt(monthStart),
		MonthEndAt:     timestampAt(monthStart.AddDate(0, 1, 0)),
		ClassID:        classID,
		AllowAll:       scope.allowAll,
		IsTeacher:      scope.isTeacher,
		TeacherID:      scope.teacherID,
		IsDuty:         scope.isDuty,
	})
	if err != nil {
		return Dashboard{}, fmt.Errorf("dashboard discipline summary: %w", err)
	}
	trend, err := s.dashboardTrend(ctx, scope, date, classID)
	if err != nil {
		return Dashboard{}, err
	}
	late, err := s.queries.DashboardLateStudents(ctx, db.DashboardLateStudentsParams{
		AttendanceDate: date,
		ClassID:        classID,
		AllowAll:       scope.allowAll,
		IsTeacher:      scope.isTeacher,
		TeacherID:      scope.teacherID,
		IsDuty:         scope.isDuty,
	})
	if err != nil {
		return Dashboard{}, fmt.Errorf("dashboard late students: %w", err)
	}
	unrecorded, err := s.queries.DashboardUnrecordedStudents(ctx, db.DashboardUnrecordedStudentsParams{
		AttendanceDate: date,
		ClassID:        classID,
		AllowAll:       scope.allowAll,
		IsTeacher:      scope.isTeacher,
		TeacherID:      scope.teacherID,
		IsDuty:         scope.isDuty,
	})
	if err != nil {
		return Dashboard{}, fmt.Errorf("dashboard unrecorded students: %w", err)
	}

	result := Dashboard{
		Date: dateString(date),
		Role: scope.role,
		Attendance: DashboardAttendance{
			TotalStudents: attendance.TotalStudents,
			Recorded:      attendance.Recorded,
			Unrecorded:    attendance.Unrecorded,
			Present:       attendance.Present,
			Late:          attendance.Late,
			Sick:          attendance.Sick,
			Excused:       attendance.Excused,
			Absent:        attendance.Absent,
		},
		Discipline: DashboardDiscipline{
			RecordsToday: discipline.RecordsToday,
			RecordsMonth: discipline.RecordsMonth,
			PointsMonth:  discipline.PointsMonth,
			NewRecords:   discipline.NewRecords,
		},
		Trend:              trend,
		LateStudents:       make([]LateStudent, 0, len(late)),
		UnrecordedStudents: make([]UnrecordedStudent, 0, len(unrecorded)),
	}
	for _, row := range late {
		result.LateStudents = append(result.LateStudents, LateStudent{
			StudentID:     uuidString(row.StudentID),
			StudentNumber: row.StudentNumber,
			StudentName:   row.FullName,
			ClassID:       uuidString(row.ClassID),
			ClassName:     row.ClassName,
			CheckInAt:     timestampPtr(row.CheckInAt),
		})
	}
	for _, row := range unrecorded {
		result.UnrecordedStudents = append(result.UnrecordedStudents, UnrecordedStudent{
			StudentID:     uuidString(row.StudentID),
			StudentNumber: row.StudentNumber,
			StudentName:   row.FullName,
			ClassID:       uuidString(row.ClassID),
			ClassName:     row.ClassName,
		})
	}
	if scope.allowAll {
		counts, err := s.queries.DashboardSystemCounts(ctx)
		if err != nil {
			return Dashboard{}, fmt.Errorf("dashboard system counts: %w", err)
		}
		activeStudents := counts.ActiveStudents
		activeTeachers := counts.ActiveTeachers
		result.ActiveStudents = &activeStudents
		result.ActiveTeachers = &activeTeachers
		// Employment-type package: school-wide active teacher breakdown by
		// employment category (counts teachers.status = 'active' server-side).
		breakdown, err := s.queries.DashboardTeacherEmploymentBreakdown(ctx)
		if err != nil {
			return Dashboard{}, fmt.Errorf("dashboard teacher employment breakdown: %w", err)
		}
		result.ActiveTeachersBreakdown = &TeacherEmploymentBreakdown{
			Permanent: breakdown.Permanent,
			Honorary:  breakdown.Honorary,
			StaffTU:   breakdown.StaffTu,
			Unknown:   breakdown.Unknown,
		}
	}
	if scope.isAdmin {
		recent, err := s.ListAuditLogs(ctx, actor, AuditLogFilter{Limit: 10})
		if err != nil {
			return Dashboard{}, err
		}
		result.RecentActivity = recent.Data
	}
	return result, nil
}

func (s *Service) dashboardDate(ctx context.Context, value string) (pgtype.Date, *time.Location, error) {
	loc, err := s.schoolLocation(ctx)
	if err != nil {
		return pgtype.Date{}, nil, fmt.Errorf("resolve school timezone for dashboard: %w", err)
	}
	if value != "" {
		date, err := parseDate(value)
		if err != nil {
			return pgtype.Date{}, nil, ErrInvalidInput
		}
		return date, loc, nil
	}
	now := time.Now().In(loc)
	return pgtype.Date{Time: time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC), Valid: true}, loc, nil
}

func (s *Service) dashboardTrend(ctx context.Context, scope actorScope, date pgtype.Date, classID pgtype.UUID) ([]TrendPoint, error) {
	from := pgtype.Date{Time: date.Time.AddDate(0, 0, -6), Valid: true}
	rows, err := s.queries.DashboardAttendanceTrend(ctx, db.DashboardAttendanceTrendParams{
		FromDate:  from,
		ToDate:    date,
		ClassID:   classID,
		AllowAll:  scope.allowAll,
		IsTeacher: scope.isTeacher,
		TeacherID: scope.teacherID,
		IsDuty:    scope.isDuty,
	})
	if err != nil {
		return nil, fmt.Errorf("dashboard attendance trend: %w", err)
	}
	byDate := make(map[string]db.DashboardAttendanceTrendRow, len(rows))
	for _, row := range rows {
		byDate[dateString(row.AttendanceDate)] = row
	}
	trend := make([]TrendPoint, 0, 7)
	for i := 0; i < 7; i++ {
		current := from.Time.AddDate(0, 0, i)
		key := current.Format("2006-01-02")
		point := TrendPoint{Date: key}
		if row, ok := byDate[key]; ok {
			point.Recorded = row.Recorded
			point.Present = row.Present
			point.Late = row.Late
			point.Sick = row.Sick
			point.Excused = row.Excused
			point.Absent = row.Absent
		}
		trend = append(trend, point)
	}
	return trend, nil
}
