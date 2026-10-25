package discipline

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

const maxExportRecords = 100000

func (s *Service) List(ctx context.Context, actor Actor, filter RecordFilter) (RecordList, error) {
	limit, offset := normalizePage(filter.Limit, filter.Offset)
	return s.list(ctx, actor, filter, limit, offset)
}

func (s *Service) list(ctx context.Context, actor Actor, filter RecordFilter, limit, offset int32) (RecordList, error) {
	from, to, err := optionalDateRange(filter.From, filter.To)
	if err != nil {
		return RecordList{}, err
	}
	studentID, err := optionalUUID(filter.StudentID)
	if err != nil {
		return RecordList{}, ErrInvalidInput
	}
	classID, err := optionalUUID(filter.ClassID)
	if err != nil {
		return RecordList{}, ErrInvalidInput
	}
	categoryID, err := optionalUUID(filter.CategoryID)
	if err != nil {
		return RecordList{}, ErrInvalidInput
	}
	recordedBy, err := optionalUUID(filter.RecordedBy)
	if err != nil {
		return RecordList{}, ErrInvalidInput
	}
	status := strings.TrimSpace(filter.Status)
	if status != "" && !validStatus(status) {
		return RecordList{}, ErrInvalidInput
	}
	scope, err := resolveScope(ctx, s.queries, actor)
	if err != nil {
		return RecordList{}, err
	}
	params := db.ListDisciplineRecordsParams{
		StudentID:  studentID,
		ClassID:    classID,
		CategoryID: categoryID,
		RecordedBy: recordedBy,
		Status:     optionalText(status),
		FromDate:   from,
		ToDate:     to,
		AllowAll:   scope.allowAll,
		IsTeacher:  scope.isTeacher,
		TeacherID:  scope.teacherID,
		IsDuty:     scope.isDuty,
		PageOffset: offset,
		PageLimit:  limit,
	}
	rows, err := s.queries.ListDisciplineRecords(ctx, params)
	if err != nil {
		return RecordList{}, fmt.Errorf("list discipline records: %w", err)
	}
	total, err := s.queries.CountDisciplineRecords(ctx, db.CountDisciplineRecordsParams{
		StudentID:  studentID,
		ClassID:    classID,
		CategoryID: categoryID,
		RecordedBy: recordedBy,
		Status:     optionalText(status),
		FromDate:   from,
		ToDate:     to,
		AllowAll:   scope.allowAll,
		IsTeacher:  scope.isTeacher,
		TeacherID:  scope.teacherID,
		IsDuty:     scope.isDuty,
	})
	if err != nil {
		return RecordList{}, fmt.Errorf("count discipline records: %w", err)
	}
	items := make([]Record, 0, len(rows))
	for _, row := range rows {
		items = append(items, recordFromListRow(row))
	}
	return RecordList{Data: items, Total: total, Limit: limit, Offset: offset}, nil
}

func (s *Service) GetRecord(ctx context.Context, actor Actor, idValue string) (RecordDetail, error) {
	id, err := parseUUID(idValue)
	if err != nil {
		return RecordDetail{}, ErrInvalidInput
	}
	row, err := s.queries.GetDisciplineRecord(ctx, id)
	if err != nil {
		if database.IsNotFound(err) {
			return RecordDetail{}, ErrNotFound
		}
		return RecordDetail{}, fmt.Errorf("get discipline record: %w", err)
	}
	if err := canReadClass(ctx, s.queries, actor, row.ClassID, row.OccurredDate); err != nil {
		return RecordDetail{}, err
	}
	followups, err := s.followups(ctx, id)
	if err != nil {
		return RecordDetail{}, err
	}
	return RecordDetail{Record: recordFromGetRow(row), Followups: followups}, nil
}

func (s *Service) CreateRecord(ctx context.Context, actor Actor, input RecordInput, requestID string) (RecordDetail, error) {
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return RecordDetail{}, ErrUnauthorized
	}
	studentID, err := parseUUID(input.StudentID)
	if err != nil {
		return RecordDetail{}, ErrInvalidInput
	}
	categoryID, err := parseUUID(input.CategoryID)
	if err != nil {
		return RecordDetail{}, ErrInvalidInput
	}
	occurredAt, occurredDate, err := s.parseOccurredAt(ctx, input.OccurredAt)
	if err != nil {
		return RecordDetail{}, err
	}
	description, err := requiredText(input.Description, 2000)
	if err != nil {
		return RecordDetail{}, err
	}
	var recordID pgtype.UUID
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		category, err := q.GetDisciplineCategory(ctx, categoryID)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("get discipline category: %w", err)
		}
		if !category.IsActive {
			return ErrInvalidInput
		}
		if category.AutoOnAttendanceStatus.Valid {
			return ErrInvalidInput
		}
		enrollment, err := q.GetStudentEnrollmentAtDate(ctx, db.GetStudentEnrollmentAtDateParams{
			StudentID:  studentID,
			OccurredAt: occurredAt,
		})
		if err != nil {
			if database.IsNotFound(err) {
				return ErrInvalidInput
			}
			return fmt.Errorf("get student enrollment: %w", err)
		}
		if err := canWriteClass(ctx, q, actor, enrollment.ClassID, occurredDate); err != nil {
			return err
		}
		points := int32(0)
		if category.DefaultPoints.Valid {
			points = category.DefaultPoints.Int32
		}
		recordID, err = q.CreateDisciplineRecord(ctx, db.CreateDisciplineRecordParams{
			StudentID:        studentID,
			EnrollmentID:     enrollment.ID,
			CategoryID:       categoryID,
			OccurredAt:       occurredAt,
			Description:      description,
			Points:           points,
			RecordedByUserID: actorID,
		})
		if err != nil {
			return mapWriteError(fmt.Errorf("create discipline record: %w", err))
		}
		if err := s.auditData(ctx, q, actorID, "discipline.record_create", "discipline_record", recordID, nil, map[string]any{
			"student_id":    input.StudentID,
			"category_id":   input.CategoryID,
			"occurred_at":   occurredAt.Time,
			"description":   description,
			"points":        points,
			"enrollment_id": uuidString(enrollment.ID),
		}, "", requestID); err != nil {
			return fmt.Errorf("write discipline record audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return RecordDetail{}, err
	}
	return s.GetRecord(ctx, actor, uuidString(recordID))
}

func (s *Service) AddFollowup(ctx context.Context, actor Actor, idValue string, input FollowupInput, requestID string) (RecordDetail, error) {
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return RecordDetail{}, ErrUnauthorized
	}
	id, err := parseUUID(idValue)
	if err != nil {
		return RecordDetail{}, ErrInvalidInput
	}
	status := strings.TrimSpace(input.Status)
	if !validStatus(status) {
		return RecordDetail{}, ErrInvalidInput
	}
	notes, err := cleanText(input.Notes, 2000)
	if err != nil {
		return RecordDetail{}, err
	}
	if status == "cancelled" && notes == "" {
		return RecordDetail{}, ErrInvalidInput
	}
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		record, err := q.GetDisciplineRecordForUpdate(ctx, id)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("get discipline record: %w", err)
		}
		if err := canWriteClass(ctx, q, actor, record.ClassID, record.OccurredDate); err != nil {
			return err
		}
		if record.Status == "cancelled" && status != "cancelled" {
			return ErrInvalidInput
		}
		if _, err := q.UpdateDisciplineRecordStatus(ctx, db.UpdateDisciplineRecordStatusParams{
			Status: status,
			ID:     id,
		}); err != nil {
			return mapWriteError(fmt.Errorf("update discipline status: %w", err))
		}
		if _, err := q.CreateDisciplineFollowup(ctx, db.CreateDisciplineFollowupParams{
			DisciplineRecordID: id,
			Status:             status,
			Notes:              optionalText(notes),
			UpdatedByUserID:    actorID,
		}); err != nil {
			return mapWriteError(fmt.Errorf("create discipline follow-up: %w", err))
		}
		if status == "cancelled" {
			if err := s.auditData(ctx, q, actorID, "discipline.cancel", "discipline_record", id, map[string]any{
				"status": record.Status,
			}, map[string]any{
				"status": "cancelled",
			}, notes, requestID); err != nil {
				return fmt.Errorf("write discipline cancel audit: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return RecordDetail{}, err
	}
	return s.GetRecord(ctx, actor, uuidString(id))
}

func (s *Service) UpdatePoints(ctx context.Context, actor Actor, idValue string, input PointsInput, requestID string) (RecordDetail, error) {
	if !hasRole(actor.Roles, "admin") {
		return RecordDetail{}, ErrForbidden
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return RecordDetail{}, ErrUnauthorized
	}
	id, err := parseUUID(idValue)
	if err != nil {
		return RecordDetail{}, ErrInvalidInput
	}
	if input.Points < 0 {
		return RecordDetail{}, ErrInvalidInput
	}
	reason, err := requiredText(input.Reason, 1000)
	if err != nil {
		return RecordDetail{}, err
	}
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		record, err := q.GetDisciplineRecordForUpdate(ctx, id)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("get discipline record: %w", err)
		}
		if record.Status == "cancelled" {
			return ErrConflict
		}
		if _, err := q.UpdateDisciplineRecordPoints(ctx, db.UpdateDisciplineRecordPointsParams{
			Points: input.Points,
			ID:     id,
		}); err != nil {
			return mapWriteError(fmt.Errorf("update discipline points: %w", err))
		}
		if err := s.auditData(ctx, q, actorID, "discipline.points_change", "discipline_record", id, map[string]any{
			"points": record.Points,
		}, map[string]any{
			"points": input.Points,
		}, reason, requestID); err != nil {
			return fmt.Errorf("write discipline points audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return RecordDetail{}, err
	}
	return s.GetRecord(ctx, actor, uuidString(id))
}

func (s *Service) StudentSummary(ctx context.Context, actor Actor, studentIDValue string) (StudentSummary, error) {
	studentID, err := parseUUID(studentIDValue)
	if err != nil {
		return StudentSummary{}, ErrInvalidInput
	}
	student, err := s.queries.GetStudent(ctx, studentID)
	if err != nil {
		if database.IsNotFound(err) {
			return StudentSummary{}, ErrNotFound
		}
		return StudentSummary{}, fmt.Errorf("get student: %w", err)
	}
	result, err := s.list(ctx, actor, RecordFilter{StudentID: studentIDValue}, maxExportRecords, 0)
	if err != nil {
		return StudentSummary{}, err
	}
	scope, err := resolveScope(ctx, s.queries, actor)
	if err != nil {
		return StudentSummary{}, err
	}
	if !scope.allowAll && len(result.Data) == 0 {
		allowed, err := s.canReadCurrentStudent(ctx, actor, studentID)
		if err != nil {
			return StudentSummary{}, err
		}
		if !allowed {
			return StudentSummary{}, ErrForbidden
		}
	}
	summary := StudentSummary{
		StudentID:     uuidString(student.ID),
		StudentNumber: student.StudentNumber,
		StudentName:   student.FullName,
		Categories:    make([]CategorySummary, 0),
		Records:       result.Data,
	}
	index := make(map[string]int)
	for _, record := range result.Data {
		if record.Status == "cancelled" {
			continue
		}
		summary.TotalRecords++
		summary.TotalPoints += record.Points
		position, exists := index[record.CategoryID]
		if !exists {
			position = len(summary.Categories)
			index[record.CategoryID] = position
			summary.Categories = append(summary.Categories, CategorySummary{
				CategoryID:   record.CategoryID,
				CategoryName: record.CategoryName,
			})
		}
		summary.Categories[position].Count++
		summary.Categories[position].Points += record.Points
	}
	// current_level: the sanction rule (sanction-rules package, migration
	// 000012) whose active point range covers the accumulated total. Null
	// when no rule matches.
	match, err := s.queries.MatchSanctionRuleForPoints(ctx, summary.TotalPoints)
	if err == nil {
		summary.CurrentLevel = &SanctionLevel{Level: textPtr(match.Level), Action: match.Action}
	} else if !database.IsNotFound(err) {
		return StudentSummary{}, fmt.Errorf("match sanction rule for points: %w", err)
	}
	return summary, nil
}

func (s *Service) canReadCurrentStudent(ctx context.Context, actor Actor, studentID pgtype.UUID) (bool, error) {
	enrollment, err := s.queries.GetCurrentStudentEnrollment(ctx, studentID)
	if err != nil {
		if database.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("get current student enrollment: %w", err)
	}
	loc, err := s.schoolLocation(ctx)
	if err != nil {
		return false, err
	}
	now := time.Now().In(loc)
	today := pgtype.Date{Time: time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC), Valid: true}
	err = canReadClass(ctx, s.queries, actor, enrollment.ClassID, today)
	if err != nil {
		if errors.Is(err, ErrForbidden) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *Service) followups(ctx context.Context, id pgtype.UUID) ([]Followup, error) {
	rows, err := s.queries.ListDisciplineFollowups(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list discipline follow-ups: %w", err)
	}
	items := make([]Followup, 0, len(rows))
	for _, row := range rows {
		items = append(items, followupFromRow(row))
	}
	return items, nil
}
