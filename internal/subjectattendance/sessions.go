package subjectattendance

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

// CreateSession is POST /attendance-sessions (admin, teacher, duty_teacher).
// A plain teacher may only open a session for their own active assignment.
func (s *Service) CreateSession(ctx context.Context, actor Actor, input SessionInput, requestID string) (Session, error) {
	scope, err := s.resolveScope(ctx, actor, true)
	if err != nil {
		return Session{}, err
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return Session{}, ErrUnauthorized
	}
	assignmentID, err := parseUUID(input.TeachingAssignmentID)
	if err != nil {
		return Session{}, ErrInvalidInput
	}
	assignment, err := s.queries.GetTeachingAssignmentContext(ctx, assignmentID)
	if err != nil {
		if database.IsNotFound(err) {
			return Session{}, ErrNotFound
		}
		return Session{}, fmt.Errorf("get assignment context: %w", err)
	}
	if !assignment.IsActive {
		return Session{}, ErrInvalidInput
	}
	if !scope.allows(assignment.TeacherID) {
		return Session{}, ErrForbidden
	}

	sessionDate, err := parseDate(input.SessionDate)
	if err != nil {
		return Session{}, ErrInvalidInput
	}
	if input.PeriodStart < 1 || input.PeriodEnd < input.PeriodStart || input.PeriodEnd > maxSessionPeriod {
		return Session{}, ErrInvalidInput
	}
	topic, err := cleanText(input.Topic, maxTopicRunes)
	if err != nil {
		return Session{}, err
	}
	notes, err := cleanText(input.Notes, maxNotesRunes)
	if err != nil {
		return Session{}, err
	}

	var created db.GetAttendanceSessionRow
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		row, err := q.CreateAttendanceSession(ctx, db.CreateAttendanceSessionParams{
			TeachingAssignmentID: assignmentID,
			SessionDate:          sessionDate,
			PeriodStart:          int16(input.PeriodStart),
			PeriodEnd:            int16(input.PeriodEnd),
			Topic:                optionalText(topic),
			Notes:                optionalText(notes),
			CreatedBy:            actorID,
		})
		if err != nil {
			return err
		}
		created, err = q.GetAttendanceSession(ctx, row.ID)
		if err != nil {
			return err
		}
		return s.audit(ctx, q, actorID, "attendance_session.create", "attendance_session",
			row.ID, nil, sessionOf(created), requestID)
	})
	if err != nil {
		return Session{}, mapWriteError(err)
	}
	return sessionOf(created), nil
}

// ListSessions is GET /attendance-sessions. from/to are mandatory; a plain
// teacher only sees sessions of their own teaching assignments.
func (s *Service) ListSessions(ctx context.Context, actor Actor, filter SessionFilter) (SessionList, error) {
	scope, err := s.resolveScope(ctx, actor, false)
	if err != nil {
		return SessionList{}, err
	}
	from, to, err := parseDateRange(filter.From, filter.To)
	if err != nil {
		return SessionList{}, err
	}
	status := strings.TrimSpace(filter.Status)
	if status != "" && !validSessionStatus(status) {
		return SessionList{}, ErrInvalidInput
	}
	params := db.ListAttendanceSessionsParams{
		FromDate: from,
		ToDate:   to,
		Status:   optionalText(status),
	}
	countParams := db.CountAttendanceSessionsParams{
		FromDate: from,
		ToDate:   to,
		Status:   optionalText(status),
	}
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
			return SessionList{}, ErrInvalidInput
		}
		switch name {
		case "class_id":
			params.ClassID, countParams.ClassID = id, id
		case "subject_id":
			params.SubjectID, countParams.SubjectID = id, id
		case "teacher_id":
			params.TeacherID, countParams.TeacherID = id, id
		}
	}
	if scope.restrict {
		params.TeacherID, countParams.TeacherID = scope.teacherID, scope.teacherID
	}
	limit, offset := normalizePage(filter.Limit, filter.Offset)
	params.PageLimit, params.PageOffset = limit, offset

	total, err := s.queries.CountAttendanceSessions(ctx, countParams)
	if err != nil {
		return SessionList{}, fmt.Errorf("count attendance sessions: %w", err)
	}
	rows, err := s.queries.ListAttendanceSessions(ctx, params)
	if err != nil {
		return SessionList{}, fmt.Errorf("list attendance sessions: %w", err)
	}
	items := make([]Session, 0, len(rows))
	for _, row := range rows {
		items = append(items, sessionOfList(row))
	}
	return SessionList{Data: items, Total: total, Limit: limit, Offset: offset}, nil
}

// GetSession is GET /attendance-sessions/:id.
func (s *Service) GetSession(ctx context.Context, actor Actor, idValue string) (Session, error) {
	scope, err := s.resolveScope(ctx, actor, false)
	if err != nil {
		return Session{}, err
	}
	row, err := s.loadSession(ctx, idValue)
	if err != nil {
		return Session{}, err
	}
	if !scope.allows(row.TeacherID) {
		return Session{}, ErrForbidden
	}
	return sessionOf(row), nil
}

// Roster is GET /attendance-sessions/:id/roster: enrolled students with their
// saved status for the session (null when not recorded yet).
func (s *Service) Roster(ctx context.Context, actor Actor, idValue string) (Roster, error) {
	scope, err := s.resolveScope(ctx, actor, false)
	if err != nil {
		return Roster{}, err
	}
	session, err := s.loadSession(ctx, idValue)
	if err != nil {
		return Roster{}, err
	}
	if !scope.allows(session.TeacherID) {
		return Roster{}, ErrForbidden
	}
	rows, err := s.queries.ListSessionRoster(ctx, session.ID)
	if err != nil {
		return Roster{}, fmt.Errorf("list session roster: %w", err)
	}
	items := make([]RosterRow, 0, len(rows))
	for _, row := range rows {
		items = append(items, RosterRow{
			StudentID:     uuidString(row.StudentID),
			StudentNumber: row.StudentNumber,
			FullName:      row.FullName,
			EnrollmentID:  uuidString(row.EnrollmentID),
			Status:        textPtr(row.Status),
			Note:          textPtr(row.Note),
		})
	}
	return Roster{Data: items}, nil
}

// Records is GET /attendance-sessions/:id/records: the saved records with
// record IDs, so the input screen can show which rows already exist.
func (s *Service) Records(ctx context.Context, actor Actor, idValue string) (SessionRecordList, error) {
	scope, err := s.resolveScope(ctx, actor, false)
	if err != nil {
		return SessionRecordList{}, err
	}
	session, err := s.loadSession(ctx, idValue)
	if err != nil {
		return SessionRecordList{}, err
	}
	if !scope.allows(session.TeacherID) {
		return SessionRecordList{}, ErrForbidden
	}
	return s.listRecords(ctx, session.ID)
}

// SaveRecords is PUT /attendance-sessions/:id/records. Every student must be
// on the session roster; a closed session rejects the write with 409.
func (s *Service) SaveRecords(ctx context.Context, actor Actor, idValue string, input SaveRecordsInput, requestID string) (SessionRecordList, error) {
	scope, err := s.resolveScope(ctx, actor, true)
	if err != nil {
		return SessionRecordList{}, err
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return SessionRecordList{}, ErrUnauthorized
	}
	if len(input.Records) == 0 || len(input.Records) > maxBulkRecords {
		return SessionRecordList{}, ErrInvalidInput
	}
	session, err := s.loadSession(ctx, idValue)
	if err != nil {
		return SessionRecordList{}, err
	}
	if !scope.allows(session.TeacherID) {
		return SessionRecordList{}, ErrForbidden
	}
	if session.Status == SessionClosed {
		return SessionRecordList{}, ErrConflict
	}

	rosterRows, err := s.queries.ListSessionRoster(ctx, session.ID)
	if err != nil {
		return SessionRecordList{}, fmt.Errorf("list session roster: %w", err)
	}
	enrollments := make(map[string]pgtype.UUID, len(rosterRows))
	for _, row := range rosterRows {
		enrollments[uuidString(row.StudentID)] = row.EnrollmentID
	}

	seen := make(map[string]struct{}, len(input.Records))
	upserts := make([]db.UpsertSessionRecordParams, 0, len(input.Records))
	for _, record := range input.Records {
		studentID, err := parseUUID(record.StudentID)
		if err != nil {
			return SessionRecordList{}, ErrInvalidInput
		}
		if !validRecordStatus(record.Status) {
			return SessionRecordList{}, ErrInvalidInput
		}
		note, err := cleanText(record.Note, maxNoteRunes)
		if err != nil {
			return SessionRecordList{}, err
		}
		key := uuidString(studentID)
		enrollmentID, enrolled := enrollments[key]
		if !enrolled {
			return SessionRecordList{}, ErrInvalidInput
		}
		if _, duplicate := seen[key]; duplicate {
			return SessionRecordList{}, ErrInvalidInput
		}
		seen[key] = struct{}{}
		upserts = append(upserts, db.UpsertSessionRecordParams{
			SessionID:    session.ID,
			StudentID:    studentID,
			EnrollmentID: enrollmentID,
			Status:       record.Status,
			Note:         optionalText(note),
			RecordedBy:   actorID,
		})
	}

	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		for _, params := range upserts {
			if _, err := q.UpsertSessionRecord(ctx, params); err != nil {
				return err
			}
		}
		return s.audit(ctx, q, actorID, "attendance_session.records", "attendance_session",
			session.ID, nil, map[string]int{"records": len(upserts)}, requestID)
	})
	if err != nil {
		return SessionRecordList{}, mapWriteError(err)
	}
	return s.listRecords(ctx, session.ID)
}

// CloseSession is POST /attendance-sessions/:id/close. Closing is idempotency
// unsafe by design: a second close answers 409, not 200.
func (s *Service) CloseSession(ctx context.Context, actor Actor, idValue string, requestID string) (Session, error) {
	scope, err := s.resolveScope(ctx, actor, true)
	if err != nil {
		return Session{}, err
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return Session{}, ErrUnauthorized
	}
	session, err := s.loadSession(ctx, idValue)
	if err != nil {
		return Session{}, err
	}
	if !scope.allows(session.TeacherID) {
		return Session{}, ErrForbidden
	}
	if session.Status == SessionClosed {
		return Session{}, ErrConflict
	}

	var closed db.GetAttendanceSessionRow
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		if _, err := q.CloseAttendanceSession(ctx, db.CloseAttendanceSessionParams{
			ClosedBy: actorID,
			ID:       session.ID,
		}); err != nil {
			if database.IsNotFound(err) {
				return ErrConflict
			}
			return err
		}
		row, err := q.GetAttendanceSession(ctx, session.ID)
		if err != nil {
			return err
		}
		closed = row
		return s.audit(ctx, q, actorID, "attendance_session.close", "attendance_session",
			session.ID, sessionOf(session), sessionOf(row), requestID)
	})
	if err != nil {
		return Session{}, mapWriteError(err)
	}
	return sessionOf(closed), nil
}

func (s *Service) loadSession(ctx context.Context, idValue string) (db.GetAttendanceSessionRow, error) {
	id, err := parseUUID(idValue)
	if err != nil {
		return db.GetAttendanceSessionRow{}, ErrInvalidInput
	}
	row, err := s.queries.GetAttendanceSession(ctx, id)
	if err != nil {
		if database.IsNotFound(err) {
			return db.GetAttendanceSessionRow{}, ErrNotFound
		}
		return db.GetAttendanceSessionRow{}, fmt.Errorf("get attendance session: %w", err)
	}
	return row, nil
}

func (s *Service) listRecords(ctx context.Context, sessionID pgtype.UUID) (SessionRecordList, error) {
	rows, err := s.queries.ListSessionRecords(ctx, sessionID)
	if err != nil {
		return SessionRecordList{}, fmt.Errorf("list session records: %w", err)
	}
	items := make([]SessionRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, SessionRecord{
			ID:            uuidString(row.ID),
			SessionID:     uuidString(row.SessionID),
			StudentID:     uuidString(row.StudentID),
			StudentNumber: row.StudentNumber,
			FullName:      row.FullName,
			EnrollmentID:  uuidString(row.EnrollmentID),
			Status:        row.Status,
			Note:          textPtr(row.Note),
			UpdatedAt:     timestamp(row.UpdatedAt),
		})
	}
	return SessionRecordList{Data: items}, nil
}

func sessionOf(row db.GetAttendanceSessionRow) Session {
	return Session{
		ID:                   uuidString(row.ID),
		TeachingAssignmentID: uuidString(row.TeachingAssignmentID),
		SessionDate:          dateString(row.SessionDate),
		PeriodStart:          int32(row.PeriodStart),
		PeriodEnd:            int32(row.PeriodEnd),
		Topic:                textPtr(row.Topic),
		Notes:                textPtr(row.Notes),
		Status:               row.Status,
		TeacherID:            uuidString(row.TeacherID),
		TeacherName:          row.TeacherName,
		SubjectID:            uuidString(row.SubjectID),
		SubjectCode:          row.SubjectCode,
		SubjectName:          row.SubjectName,
		ClassID:              uuidString(row.ClassID),
		ClassName:            row.ClassName,
		ClosedAt:             timestampPtr(row.ClosedAt),
		CreatedAt:            timestamp(row.CreatedAt),
		UpdatedAt:            timestamp(row.UpdatedAt),
	}
}

func sessionOfList(row db.ListAttendanceSessionsRow) Session {
	return Session{
		ID:                   uuidString(row.ID),
		TeachingAssignmentID: uuidString(row.TeachingAssignmentID),
		SessionDate:          dateString(row.SessionDate),
		PeriodStart:          int32(row.PeriodStart),
		PeriodEnd:            int32(row.PeriodEnd),
		Topic:                textPtr(row.Topic),
		Notes:                textPtr(row.Notes),
		Status:               row.Status,
		TeacherID:            uuidString(row.TeacherID),
		TeacherName:          row.TeacherName,
		SubjectID:            uuidString(row.SubjectID),
		SubjectCode:          row.SubjectCode,
		SubjectName:          row.SubjectName,
		ClassID:              uuidString(row.ClassID),
		ClassName:            row.ClassName,
		ClosedAt:             timestampPtr(row.ClosedAt),
		CreatedAt:            timestamp(row.CreatedAt),
		UpdatedAt:            timestamp(row.UpdatedAt),
	}
}
