package attendance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

const maxReportDays = 366

func parseUUID(value string) (pgtype.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: id, Valid: true}, nil
}

func optionalUUID(value string) (pgtype.UUID, error) {
	if strings.TrimSpace(value) == "" {
		return pgtype.UUID{}, nil
	}
	return parseUUID(value)
}

func parseDate(value string) (pgtype.Date, error) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	if err != nil {
		return pgtype.Date{}, err
	}
	return pgtype.Date{Time: t, Valid: true}, nil
}

func dateString(value pgtype.Date) string {
	if !value.Valid {
		return ""
	}
	return value.Time.Format("2006-01-02")
}

func todayIn(loc *time.Location) time.Time {
	now := time.Now().In(loc)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

func optionalText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}

func textString(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func timestamp(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}

func timestampPtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time
	return &t
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}

func cleanText(value string, max int) (string, error) {
	value = strings.TrimSpace(value)
	if strings.IndexByte(value, 0) >= 0 || utf8.RuneCountInString(value) > max {
		return "", ErrInvalidInput
	}
	return value, nil
}

func requiredText(value string, max int) (string, error) {
	value, err := cleanText(value, max)
	if err != nil || value == "" {
		return "", ErrInvalidInput
	}
	return value, nil
}

func validStatus(value string) bool {
	switch value {
	case "present", "late", "sick", "excused", "absent":
		return true
	default:
		return false
	}
}

func parseTimeOfDay(value string) (pgtype.Time, error) {
	value = strings.TrimSpace(value)
	var parsed time.Time
	var err error
	for _, layout := range []string{"15:04", "15:04:05"} {
		parsed, err = time.Parse(layout, value)
		if err == nil {
			break
		}
	}
	if err != nil {
		return pgtype.Time{}, ErrInvalidInput
	}
	micros := int64(parsed.Hour())*3600_000_000 +
		int64(parsed.Minute())*60_000_000 +
		int64(parsed.Second())*1_000_000
	return pgtype.Time{Microseconds: micros, Valid: true}, nil
}

func timeOfDayString(value pgtype.Time) string {
	if !value.Valid {
		return ""
	}
	d := time.Duration(value.Microseconds) * time.Microsecond
	return fmt.Sprintf("%02d:%02d", int(d.Hours()), int(d.Minutes())%60)
}

func parseCheckIn(value string, date pgtype.Date, loc *time.Location) (pgtype.Timestamptz, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return pgtype.Timestamptz{}, nil
	}
	if strings.Contains(value, ":") && !strings.Contains(value, "T") && len(value) <= 8 {
		clock, err := parseTimeOfDay(value)
		if err != nil {
			return pgtype.Timestamptz{}, err
		}
		seconds := clock.Microseconds / 1_000_000
		local := time.Date(date.Time.Year(), date.Time.Month(), date.Time.Day(),
			int(seconds/3600), int((seconds%3600)/60), int(seconds%60), 0, loc)
		return pgtype.Timestamptz{Time: local.UTC(), Valid: true}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return pgtype.Timestamptz{}, ErrInvalidInput
	}
	local := parsed.In(loc)
	if local.Format("2006-01-02") != dateString(date) {
		return pgtype.Timestamptz{}, ErrInvalidInput
	}
	return pgtype.Timestamptz{Time: parsed.UTC(), Valid: true}, nil
}

func effectiveDay(row db.GetClassDayContextRow) bool {
	return row.Effective
}

func summaryFromRows(rows []db.ListAttendanceRosterRow) Summary {
	summary := Summary{Total: len(rows)}
	for _, row := range rows {
		if !row.Status.Valid {
			summary.Unrecorded++
			continue
		}
		summary.Recorded++
		switch row.Status.String {
		case "present":
			summary.Present++
		case "late":
			summary.Late++
		case "sick":
			summary.Sick++
		case "excused":
			summary.Excused++
		case "absent":
			summary.Absent++
		}
	}
	return summary
}

type actorScope struct {
	allowAll  bool
	isTeacher bool
	isDuty    bool
	teacherID pgtype.UUID
}

func resolveScope(ctx context.Context, q *db.Queries, actor Actor) (actorScope, error) {
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return actorScope{}, ErrUnauthorized
	}
	scope := actorScope{}
	if hasRole(actor.Roles, "admin") || hasRole(actor.Roles, "principal") {
		scope.allowAll = true
	}
	if !hasRole(actor.Roles, "teacher") && !hasRole(actor.Roles, "duty_teacher") {
		return scope, nil
	}
	teacher, err := q.GetTeacherByUserID(ctx, actorID)
	if err != nil {
		if database.IsNotFound(err) {
			return scope, nil
		}
		return actorScope{}, fmt.Errorf("find teacher for attendance scope: %w", err)
	}
	scope.teacherID = teacher.ID
	scope.isTeacher = hasRole(actor.Roles, "teacher")
	scope.isDuty = hasRole(actor.Roles, "duty_teacher")
	return scope, nil
}

func hasRole(roles []string, role string) bool {
	for _, candidate := range roles {
		if candidate == role {
			return true
		}
	}
	return false
}

func canReadClass(ctx context.Context, q *db.Queries, actor Actor, classID pgtype.UUID, date pgtype.Date) error {
	scope, err := resolveScope(ctx, q, actor)
	if err != nil {
		return err
	}
	if scope.allowAll {
		return nil
	}
	if !scope.teacherID.Valid {
		return ErrForbidden
	}
	if scope.isTeacher {
		homeroom, err := q.HasHomeroomAssignmentForDate(ctx, db.HasHomeroomAssignmentForDateParams{
			ClassID:        classID,
			TeacherID:      scope.teacherID,
			AttendanceDate: date,
		})
		if err != nil {
			return fmt.Errorf("check homeroom scope: %w", err)
		}
		if homeroom {
			return nil
		}
	}
	if scope.isDuty {
		duty, err := q.HasDutyAssignmentForDate(ctx, db.HasDutyAssignmentForDateParams{
			TeacherID:      scope.teacherID,
			AttendanceDate: date,
		})
		if err != nil {
			return fmt.Errorf("check duty scope: %w", err)
		}
		if duty {
			return nil
		}
	}
	return ErrForbidden
}

func canWriteClass(ctx context.Context, q *db.Queries, actor Actor, classID pgtype.UUID, date pgtype.Date) error {
	if hasRole(actor.Roles, "admin") {
		return nil
	}
	if !hasRole(actor.Roles, "teacher") && !hasRole(actor.Roles, "duty_teacher") {
		return ErrForbidden
	}
	return canReadClass(ctx, q, actor, classID, date)
}

func normalizePage(limit, offset int32) (int32, int32) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func validateDateRange(from, to string) (pgtype.Date, pgtype.Date, error) {
	start, err := parseDate(from)
	if err != nil {
		return pgtype.Date{}, pgtype.Date{}, ErrInvalidInput
	}
	end, err := parseDate(to)
	if err != nil || end.Time.Before(start.Time) {
		return pgtype.Date{}, pgtype.Date{}, ErrInvalidInput
	}
	if int(end.Time.Sub(start.Time).Hours()/24)+1 > maxReportDays {
		return pgtype.Date{}, pgtype.Date{}, ErrInvalidInput
	}
	return start, end, nil
}

func mapWriteError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, ErrUnauthorized),
		errors.Is(err, ErrForbidden),
		errors.Is(err, ErrInvalidInput),
		errors.Is(err, ErrNotFound),
		errors.Is(err, ErrConflict):
		return err
	case database.IsUniqueViolation(err), database.IsExclusionViolation(err):
		return ErrConflict
	case database.IsForeignKeyViolation(err),
		database.IsCheckViolation(err),
		database.IsNotNullViolation(err):
		return ErrInvalidInput
	default:
		return err
	}
}

func (s *Service) auditData(ctx context.Context, q *db.Queries, actorID pgtype.UUID, action, entityType string, entityID pgtype.UUID, before, after any, requestID string) error {
	var beforeData, afterData []byte
	var err error
	if before != nil {
		beforeData, err = json.Marshal(before)
		if err != nil {
			return fmt.Errorf("encode attendance audit before data: %w", err)
		}
	}
	if after != nil {
		afterData, err = json.Marshal(after)
		if err != nil {
			return fmt.Errorf("encode attendance audit after data: %w", err)
		}
	}
	_, err = q.CreateAuditLog(ctx, db.CreateAuditLogParams{
		ActorUserID: actorID,
		Action:      action,
		EntityType:  entityType,
		EntityID:    entityID,
		RequestID:   optionalText(requestID),
		BeforeData:  string(beforeData),
		AfterData:   string(afterData),
	})
	return err
}

func settingsFromRow(row db.SchoolSetting) Settings {
	return Settings{
		SchoolName:    row.SchoolName,
		Timezone:      row.Timezone,
		AutoCloseTime: timeOfDayString(row.AttendanceAutoCloseTime),
		UpdatedAt:     timestamp(row.UpdatedAt),
	}
}

func scheduleFromRow(row db.SchoolSchedule) Schedule {
	return Schedule{
		Weekday:              row.Weekday,
		StartTime:            timeOfDayString(row.StartTime),
		LateToleranceMinutes: row.LateToleranceMinutes,
	}
}

func holidayFromRow(row db.SchoolHoliday) Holiday {
	return Holiday{
		Date:      dateString(row.HolidayDate),
		Name:      row.Name,
		CreatedAt: timestamp(row.CreatedAt),
	}
}

func closureFromRow(row db.GetAttendanceClosureRow) Closure {
	return Closure{
		ID:           uuidString(row.ID),
		ClassID:      uuidString(row.ClassID),
		Date:         dateString(row.AttendanceDate),
		CloseType:    row.CloseType,
		ClosedByID:   uuidString(row.ClosedByUserID),
		ClosedByName: row.ClosedByName,
		ClosedAt:     timestamp(row.ClosedAt),
		AbsentFilled: row.AbsentFilled,
	}
}

func closureFromCreated(row db.AttendanceDayClosure, closedByName string) Closure {
	return Closure{
		ID:           uuidString(row.ID),
		ClassID:      uuidString(row.ClassID),
		Date:         dateString(row.AttendanceDate),
		CloseType:    row.CloseType,
		ClosedByID:   uuidString(row.ClosedByUserID),
		ClosedByName: closedByName,
		ClosedAt:     timestamp(row.ClosedAt),
		AbsentFilled: row.AbsentFilled,
	}
}

func rosterStudent(row db.ListAttendanceRosterRow) RosterStudent {
	student := RosterStudent{
		StudentID:     uuidString(row.StudentID),
		StudentNumber: row.StudentNumber,
		FullName:      row.FullName,
		EnrollmentID:  uuidString(row.EnrollmentID),
	}
	if row.AttendanceID.Valid {
		record := Record{
			ID:               uuidString(row.AttendanceID),
			StudentID:        uuidString(row.StudentID),
			EnrollmentID:     uuidString(row.EnrollmentID),
			Status:           row.Status.String,
			CheckInAt:        timestampPtr(row.CheckInAt),
			Notes:            textString(row.Notes),
			Source:           row.Source.String,
			RecordedByUserID: uuidString(row.RecordedByUserID),
			UpdatedByUserID:  uuidString(row.UpdatedByUserID),
			CreatedAt:        timestamp(row.CreatedAt),
			UpdatedAt:        timestamp(row.UpdatedAt),
		}
		student.Record = &record
	}
	return student
}

func historyFromRow(row db.ListAttendanceHistoryRow) HistoryRecord {
	return HistoryRecord{
		ID:               uuidString(row.ID),
		Date:             dateString(row.AttendanceDate),
		Status:           row.Status,
		CheckInAt:        timestampPtr(row.CheckInAt),
		Notes:            textString(row.Notes),
		Source:           row.Source,
		StudentID:        uuidString(row.StudentID),
		StudentNumber:    row.StudentNumber,
		StudentName:      row.FullName,
		ClassID:          uuidString(row.ClassID),
		ClassName:        row.ClassName,
		AcademicYearID:   uuidString(row.AcademicYearID),
		AcademicYearName: row.AcademicYearName,
		Closed:           row.ClosureID.Valid,
		CreatedAt:        timestamp(row.CreatedAt),
		UpdatedAt:        timestamp(row.UpdatedAt),
	}
}

func summaryFromRow(row db.ListAttendanceSummaryRow) StudentSummary {
	percentage := 0.0
	if row.TotalDays > 0 {
		percentage = float64(row.PresentDays+row.LateDays) / float64(row.TotalDays) * 100
		percentage = float64(int(percentage*100+0.5)) / 100
	}
	return StudentSummary{
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
		Percentage:    percentage,
	}
}

func correctionFromRow(row db.AttendanceCorrection) Correction {
	return Correction{
		ID:            uuidString(row.ID),
		RecordID:      uuidString(row.AttendanceRecordID),
		OldStatus:     row.OldStatus,
		NewStatus:     row.NewStatus,
		OldCheckInAt:  timestampPtr(row.OldCheckInAt),
		NewCheckInAt:  timestampPtr(row.NewCheckInAt),
		OldNotes:      textString(row.OldNotes),
		NewNotes:      textString(row.NewNotes),
		Reason:        row.Reason,
		CorrectedByID: uuidString(row.CorrectedByUserID),
		CorrectedAt:   timestamp(row.CorrectedAt),
	}
}

func recordFromDetailRow(row db.GetAttendanceRecordRow) Record {
	record := Record{
		ID:               uuidString(row.ID),
		StudentID:        uuidString(row.StudentID),
		EnrollmentID:     uuidString(row.EnrollmentID),
		Date:             dateString(row.AttendanceDate),
		Status:           row.Status,
		CheckInAt:        timestampPtr(row.CheckInAt),
		Notes:            textString(row.Notes),
		Source:           row.Source,
		RecordedByUserID: uuidString(row.RecordedByUserID),
		UpdatedByUserID:  uuidString(row.UpdatedByUserID),
		CreatedAt:        timestamp(row.CreatedAt),
		UpdatedAt:        timestamp(row.UpdatedAt),
		StudentNumber:    row.StudentNumber,
		StudentName:      row.FullName,
		ClassID:          uuidString(row.ClassID),
		ClassName:        row.ClassName,
	}
	return record
}
