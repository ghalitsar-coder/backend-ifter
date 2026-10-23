package attendance

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

const maxBulkRecords = 500

func (s *Service) location(ctx context.Context) (*time.Location, error) {
	settings, err := s.queries.GetSchoolSettings(ctx)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get school settings: %w", err)
	}
	loc, err := time.LoadLocation(settings.Timezone)
	if err != nil {
		return nil, fmt.Errorf("load school timezone: %w", err)
	}
	return loc, nil
}

func (s *Service) resolveDate(ctx context.Context, value string) (pgtype.Date, *time.Location, error) {
	loc, err := s.location(ctx)
	if err != nil {
		return pgtype.Date{}, nil, err
	}
	if strings.TrimSpace(value) == "" {
		return pgtype.Date{Time: todayIn(loc), Valid: true}, loc, nil
	}
	date, err := parseDate(value)
	if err != nil {
		return pgtype.Date{}, nil, ErrInvalidInput
	}
	return date, loc, nil
}

func (s *Service) GetRoster(ctx context.Context, actor Actor, classIDValue, dateValue string) (Roster, error) {
	classID, err := parseUUID(classIDValue)
	if err != nil {
		return Roster{}, ErrInvalidInput
	}
	date, _, err := s.resolveDate(ctx, dateValue)
	if err != nil {
		return Roster{}, err
	}
	classDay, err := s.queries.GetClassDayContext(ctx, db.GetClassDayContextParams{
		AttendanceDate: date,
		ClassID:        classID,
	})
	if err != nil {
		if database.IsNotFound(err) {
			return Roster{}, ErrNotFound
		}
		return Roster{}, fmt.Errorf("get class day: %w", err)
	}
	if err := canReadClass(ctx, s.queries, actor, classID, date); err != nil {
		return Roster{}, err
	}
	closure, err := s.getClosure(ctx, s.queries, classID, date)
	if err != nil {
		return Roster{}, err
	}
	rows, err := s.queries.ListAttendanceRoster(ctx, db.ListAttendanceRosterParams{
		AttendanceDate: date,
		ClassID:        classID,
	})
	if err != nil {
		return Roster{}, fmt.Errorf("list attendance roster: %w", err)
	}
	students := make([]RosterStudent, 0, len(rows))
	for _, row := range rows {
		students = append(students, rosterStudent(row))
	}
	return Roster{
		Class: ClassRef{
			ID:               uuidString(classDay.ID),
			Name:             classDay.Name,
			AcademicYearID:   uuidString(classDay.AcademicYearID),
			AcademicYearName: classDay.AcademicYearName,
		},
		Date:      dateString(date),
		Effective: effectiveDay(classDay),
		Closure:   closure,
		Summary:   summaryFromRows(rows),
		Students:  students,
	}, nil
}

func (s *Service) getClosure(ctx context.Context, q *db.Queries, classID pgtype.UUID, date pgtype.Date) (*Closure, error) {
	row, err := q.GetAttendanceClosure(ctx, db.GetAttendanceClosureParams{
		ClassID:        classID,
		AttendanceDate: date,
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get attendance closure: %w", err)
	}
	closure := closureFromRow(row)
	return &closure, nil
}

type preparedRecord struct {
	studentID pgtype.UUID
	status    string
	checkInAt pgtype.Timestamptz
	notes     pgtype.Text
}

func (s *Service) Save(ctx context.Context, actor Actor, input SaveInput, requestID string) (Roster, error) {
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return Roster{}, ErrUnauthorized
	}
	classID, err := parseUUID(input.ClassID)
	if err != nil {
		return Roster{}, ErrInvalidInput
	}
	date, loc, err := s.resolveDate(ctx, input.Date)
	if err != nil {
		return Roster{}, err
	}
	if len(input.Records) == 0 || len(input.Records) > maxBulkRecords {
		return Roster{}, ErrInvalidInput
	}
	prepared := make([]preparedRecord, 0, len(input.Records))
	seen := make(map[string]struct{}, len(input.Records))
	for _, item := range input.Records {
		studentID, err := parseUUID(item.StudentID)
		if err != nil {
			return Roster{}, ErrInvalidInput
		}
		if _, exists := seen[uuidString(studentID)]; exists {
			return Roster{}, ErrInvalidInput
		}
		seen[uuidString(studentID)] = struct{}{}
		status := strings.TrimSpace(item.Status)
		if !validStatus(status) {
			return Roster{}, ErrInvalidInput
		}
		notes, err := cleanText(item.Notes, 2000)
		if err != nil {
			return Roster{}, err
		}
		checkIn, err := parseCheckIn(item.CheckInAt, date, loc)
		if err != nil {
			return Roster{}, err
		}
		if status != "present" && status != "late" && checkIn.Valid {
			return Roster{}, ErrInvalidInput
		}
		prepared = append(prepared, preparedRecord{
			studentID: studentID,
			status:    status,
			checkInAt: checkIn,
			notes:     optionalText(notes),
		})
	}

	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		classDay, err := q.GetClassDayContext(ctx, db.GetClassDayContextParams{
			AttendanceDate: date,
			ClassID:        classID,
		})
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("get class day: %w", err)
		}
		if err := canWriteClass(ctx, q, actor, classID, date); err != nil {
			return err
		}
		if classDay.Status != "active" || !classDay.AcademicYearActive {
			return ErrInvalidInput
		}
		if !effectiveDay(classDay) {
			return ErrInvalidInput
		}
		if date.Time.After(todayIn(loc)) {
			return ErrInvalidInput
		}
		closure, err := q.GetAttendanceClosure(ctx, db.GetAttendanceClosureParams{
			ClassID:        classID,
			AttendanceDate: date,
		})
		if err == nil {
			_ = closure
			return ErrConflict
		}
		if !database.IsNotFound(err) {
			return fmt.Errorf("check attendance closure: %w", err)
		}
		roster, err := q.ListAttendanceRoster(ctx, db.ListAttendanceRosterParams{
			AttendanceDate: date,
			ClassID:        classID,
		})
		if err != nil {
			return fmt.Errorf("list attendance roster: %w", err)
		}
		enrollments := make(map[string]pgtype.UUID, len(roster))
		for _, row := range roster {
			enrollments[uuidString(row.StudentID)] = row.EnrollmentID
		}
		for _, item := range prepared {
			enrollmentID, exists := enrollments[uuidString(item.studentID)]
			if !exists {
				return ErrInvalidInput
			}
			_, err := q.UpsertAttendanceRecord(ctx, db.UpsertAttendanceRecordParams{
				StudentID:      item.studentID,
				EnrollmentID:   enrollmentID,
				AttendanceDate: date,
				Status:         item.status,
				CheckInAt:      item.checkInAt,
				Notes:          item.notes,
				ActorUserID:    actorID,
			})
			if err != nil {
				if database.IsNotFound(err) {
					return ErrConflict
				}
				return mapWriteError(fmt.Errorf("save attendance record: %w", err))
			}
		}
		return nil
	})
	if err != nil {
		return Roster{}, err
	}
	return s.GetRoster(ctx, actor, input.ClassID, dateString(date))
}

func (s *Service) Close(ctx context.Context, actor Actor, classIDValue, dateValue, requestID string) (CloseResult, error) {
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return CloseResult{}, ErrUnauthorized
	}
	classID, err := parseUUID(classIDValue)
	if err != nil {
		return CloseResult{}, ErrInvalidInput
	}
	date, loc, err := s.resolveDate(ctx, dateValue)
	if err != nil {
		return CloseResult{}, err
	}
	_, summary, err := s.closeDay(ctx, classID, date, loc, "manual", actorID, &actor)
	if err != nil {
		return CloseResult{}, err
	}
	roster, err := s.GetRoster(ctx, actor, classIDValue, dateString(date))
	if err != nil {
		return CloseResult{}, err
	}
	if roster.Closure == nil {
		return CloseResult{}, fmt.Errorf("attendance closure missing after close")
	}
	return CloseResult{Closure: *roster.Closure, Summary: summary}, nil
}

func (s *Service) closeDay(ctx context.Context, classID pgtype.UUID, date pgtype.Date, loc *time.Location, closeType string, closedBy pgtype.UUID, actor *Actor) (Closure, Summary, error) {
	var closure db.AttendanceDayClosure
	var summary Summary
	err := database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		classDay, err := q.GetClassDayContext(ctx, db.GetClassDayContextParams{
			AttendanceDate: date,
			ClassID:        classID,
		})
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("get class day: %w", err)
		}
		if actor != nil {
			if err := canWriteClass(ctx, q, *actor, classID, date); err != nil {
				return err
			}
			if classDay.Status != "active" || !classDay.AcademicYearActive {
				return ErrInvalidInput
			}
			if date.Time.After(todayIn(loc)) {
				return ErrInvalidInput
			}
		}
		if !effectiveDay(classDay) {
			return ErrInvalidInput
		}
		if _, err := q.GetAttendanceClosure(ctx, db.GetAttendanceClosureParams{
			ClassID:        classID,
			AttendanceDate: date,
		}); err == nil {
			return ErrConflict
		} else if !database.IsNotFound(err) {
			return fmt.Errorf("check attendance closure: %w", err)
		}
		systemID, err := q.GetSystemUserID(ctx)
		if err != nil {
			return fmt.Errorf("get system user: %w", err)
		}
		closure, err = q.CreateAttendanceClosure(ctx, db.CreateAttendanceClosureParams{
			ClassID:        classID,
			AttendanceDate: date,
			CloseType:      closeType,
			ClosedByUserID: closedBy,
		})
		if err != nil {
			return mapWriteError(fmt.Errorf("create attendance closure: %w", err))
		}
		filled, err := q.FillMissingAttendanceAbsent(ctx, db.FillMissingAttendanceAbsentParams{
			AttendanceDate: date,
			ClassID:        classID,
			SystemUserID:   systemID,
		})
		if err != nil {
			return mapWriteError(fmt.Errorf("fill missing attendance: %w", err))
		}
		if err := q.UpdateAttendanceClosureFilled(ctx, db.UpdateAttendanceClosureFilledParams{
			AbsentFilled: int32(filled),
			ID:           closure.ID,
		}); err != nil {
			return fmt.Errorf("update closure filled count: %w", err)
		}
		closure.AbsentFilled = int32(filled)
		rows, err := q.ListAttendanceRoster(ctx, db.ListAttendanceRosterParams{
			AttendanceDate: date,
			ClassID:        classID,
		})
		if err != nil {
			return fmt.Errorf("list attendance roster after close: %w", err)
		}
		summary = summaryFromRows(rows)
		return nil
	})
	if err != nil {
		return Closure{}, Summary{}, err
	}
	return closureFromCreated(closure, ""), summary, nil
}

func (s *Service) Correct(ctx context.Context, actor Actor, recordIDValue string, input CorrectionInput, requestID string) (RecordDetail, error) {
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return RecordDetail{}, ErrUnauthorized
	}
	recordID, err := parseUUID(recordIDValue)
	if err != nil {
		return RecordDetail{}, ErrInvalidInput
	}
	loc, err := s.location(ctx)
	if err != nil {
		return RecordDetail{}, err
	}
	reason, err := requiredText(input.Reason, 1000)
	if err != nil {
		return RecordDetail{}, err
	}
	var nextStatus string
	if input.Status != nil {
		nextStatus = strings.TrimSpace(*input.Status)
		if !validStatus(nextStatus) {
			return RecordDetail{}, ErrInvalidInput
		}
	}
	var nextNotes pgtype.Text
	notesSet := input.Notes != nil
	if notesSet {
		notes, err := cleanText(*input.Notes, 2000)
		if err != nil {
			return RecordDetail{}, err
		}
		nextNotes = optionalText(notes)
	}

	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		record, err := q.GetAttendanceRecordForUpdate(ctx, recordID)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("get attendance record: %w", err)
		}
		if err := canWriteClass(ctx, q, actor, record.ClassID, record.AttendanceDate); err != nil {
			return err
		}
		if _, err := q.GetAttendanceClosure(ctx, db.GetAttendanceClosureParams{
			ClassID:        record.ClassID,
			AttendanceDate: record.AttendanceDate,
		}); err != nil {
			if database.IsNotFound(err) {
				return ErrConflict
			}
			return fmt.Errorf("check attendance closure: %w", err)
		}
		newStatus := record.Status
		if input.Status != nil {
			newStatus = nextStatus
		}
		newCheckIn := record.CheckInAt
		if input.CheckInAt != nil {
			newCheckIn, err = parseCheckIn(*input.CheckInAt, record.AttendanceDate, loc)
			if err != nil {
				return err
			}
		}
		newNotes := record.Notes
		if notesSet {
			newNotes = nextNotes
		}
		if newStatus != "present" && newStatus != "late" && newCheckIn.Valid {
			return ErrInvalidInput
		}
		if _, err := q.CreateAttendanceCorrection(ctx, db.CreateAttendanceCorrectionParams{
			AttendanceRecordID: record.ID,
			OldStatus:          record.Status,
			NewStatus:          newStatus,
			OldCheckInAt:       record.CheckInAt,
			NewCheckInAt:       newCheckIn,
			OldNotes:           record.Notes,
			NewNotes:           newNotes,
			Reason:             reason,
			CorrectedByUserID:  actorID,
		}); err != nil {
			return mapWriteError(fmt.Errorf("create attendance correction: %w", err))
		}
		if _, err := q.UpdateAttendanceRecord(ctx, db.UpdateAttendanceRecordParams{
			Status:      newStatus,
			CheckInAt:   newCheckIn,
			Notes:       newNotes,
			ActorUserID: actorID,
			ID:          record.ID,
		}); err != nil {
			return mapWriteError(fmt.Errorf("update attendance record: %w", err))
		}
		return nil
	})
	if err != nil {
		return RecordDetail{}, err
	}
	return s.GetRecord(ctx, actor, recordIDValue)
}

func (s *Service) GetRecord(ctx context.Context, actor Actor, recordIDValue string) (RecordDetail, error) {
	recordID, err := parseUUID(recordIDValue)
	if err != nil {
		return RecordDetail{}, ErrInvalidInput
	}
	row, err := s.queries.GetAttendanceRecord(ctx, recordID)
	if err != nil {
		if database.IsNotFound(err) {
			return RecordDetail{}, ErrNotFound
		}
		return RecordDetail{}, fmt.Errorf("get attendance record: %w", err)
	}
	if err := canReadClass(ctx, s.queries, actor, row.ClassID, row.AttendanceDate); err != nil {
		return RecordDetail{}, err
	}
	corrections, err := s.queries.ListAttendanceCorrections(ctx, recordID)
	if err != nil {
		return RecordDetail{}, fmt.Errorf("list attendance corrections: %w", err)
	}
	items := make([]Correction, 0, len(corrections))
	for _, correction := range corrections {
		items = append(items, correctionFromRow(correction))
	}
	return RecordDetail{Record: recordFromDetailRow(row), Corrections: items}, nil
}
