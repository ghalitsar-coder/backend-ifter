-- Per-subject attendance (migration 000020). Subject and teaching-assignment
-- master data, lesson sessions, per-student session records, and the recap.

-- name: ListSubjects :many
SELECT id, code, name, description, is_active, created_at, updated_at
FROM subjects
WHERE (
    sqlc.narg(search)::text IS NULL
    OR name ILIKE '%' || sqlc.narg(search)::text || '%'
    OR code ILIKE '%' || sqlc.narg(search)::text || '%'
  )
  AND (sqlc.narg(is_active)::boolean IS NULL OR is_active = sqlc.narg(is_active)::boolean)
ORDER BY name, id;

-- name: GetSubject :one
SELECT id, code, name, description, is_active, created_at, updated_at
FROM subjects
WHERE id = sqlc.arg(id);

-- name: CreateSubject :one
INSERT INTO subjects (code, name, description, is_active)
VALUES (sqlc.arg(code), sqlc.arg(name), sqlc.narg(description), sqlc.arg(is_active))
RETURNING id, code, name, description, is_active, created_at, updated_at;

-- name: UpdateSubject :one
UPDATE subjects
SET code        = sqlc.arg(code),
    name        = sqlc.arg(name),
    description = sqlc.narg(description),
    is_active   = sqlc.arg(is_active),
    updated_at  = now()
WHERE id = sqlc.arg(id)
RETURNING id, code, name, description, is_active, created_at, updated_at;

-- name: ListTeachingAssignments :many
SELECT ta.id, ta.teacher_id, t.full_name AS teacher_name,
       ta.subject_id, s.code AS subject_code, s.name AS subject_name,
       ta.class_id, c.name AS class_name, ta.is_active, ta.created_at, ta.updated_at
FROM teaching_assignments ta
JOIN teachers t ON t.id = ta.teacher_id
JOIN subjects s ON s.id = ta.subject_id
JOIN classes c ON c.id = ta.class_id
WHERE (sqlc.narg(teacher_id)::uuid IS NULL OR ta.teacher_id = sqlc.narg(teacher_id)::uuid)
  AND (sqlc.narg(subject_id)::uuid IS NULL OR ta.subject_id = sqlc.narg(subject_id)::uuid)
  AND (sqlc.narg(class_id)::uuid IS NULL OR ta.class_id = sqlc.narg(class_id)::uuid)
  AND (sqlc.narg(is_active)::boolean IS NULL OR ta.is_active = sqlc.narg(is_active)::boolean)
ORDER BY c.name, s.name, t.full_name, ta.id;

-- name: GetTeachingAssignment :one
SELECT ta.id, ta.teacher_id, t.full_name AS teacher_name,
       ta.subject_id, s.code AS subject_code, s.name AS subject_name,
       ta.class_id, c.name AS class_name, ta.is_active, ta.created_at, ta.updated_at
FROM teaching_assignments ta
JOIN teachers t ON t.id = ta.teacher_id
JOIN subjects s ON s.id = ta.subject_id
JOIN classes c ON c.id = ta.class_id
WHERE ta.id = sqlc.arg(id);

-- name: GetTeachingAssignmentContext :one
SELECT ta.id, ta.teacher_id, ta.subject_id, ta.class_id, ta.is_active,
       t.user_id AS teacher_user_id
FROM teaching_assignments ta
JOIN teachers t ON t.id = ta.teacher_id
WHERE ta.id = sqlc.arg(id);

-- name: CreateTeachingAssignment :one
INSERT INTO teaching_assignments (teacher_id, subject_id, class_id, is_active, created_by)
VALUES (sqlc.arg(teacher_id), sqlc.arg(subject_id), sqlc.arg(class_id),
        sqlc.arg(is_active), sqlc.arg(created_by))
RETURNING id, teacher_id, subject_id, class_id, is_active, created_at, updated_at;

-- name: UpdateTeachingAssignment :one
UPDATE teaching_assignments
SET teacher_id = sqlc.arg(teacher_id),
    subject_id = sqlc.arg(subject_id),
    class_id   = sqlc.arg(class_id),
    is_active  = sqlc.arg(is_active),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id, teacher_id, subject_id, class_id, is_active, created_at, updated_at;

-- name: CreateAttendanceSession :one
INSERT INTO attendance_sessions
  (teaching_assignment_id, session_date, period_start, period_end, topic, notes, created_by)
VALUES
  (sqlc.arg(teaching_assignment_id), sqlc.arg(session_date), sqlc.arg(period_start),
   sqlc.arg(period_end), sqlc.narg(topic), sqlc.narg(notes), sqlc.arg(created_by))
RETURNING id, teaching_assignment_id, session_date, period_start, period_end, topic,
          notes, status, created_by, closed_by, closed_at, created_at, updated_at;

-- name: GetAttendanceSession :one
SELECT ses.id, ses.teaching_assignment_id, ses.session_date, ses.period_start,
       ses.period_end, ses.topic, ses.notes, ses.status, ses.created_by,
       ses.closed_by, ses.closed_at, ses.created_at, ses.updated_at,
       ta.teacher_id, t.full_name AS teacher_name,
       ta.subject_id, s.code AS subject_code, s.name AS subject_name,
       ta.class_id, c.name AS class_name
FROM attendance_sessions ses
JOIN teaching_assignments ta ON ta.id = ses.teaching_assignment_id
JOIN teachers t ON t.id = ta.teacher_id
JOIN subjects s ON s.id = ta.subject_id
JOIN classes c ON c.id = ta.class_id
WHERE ses.id = sqlc.arg(id);

-- name: ListAttendanceSessions :many
SELECT ses.id, ses.teaching_assignment_id, ses.session_date, ses.period_start,
       ses.period_end, ses.topic, ses.notes, ses.status, ses.created_by,
       ses.closed_by, ses.closed_at, ses.created_at, ses.updated_at,
       ta.teacher_id, t.full_name AS teacher_name,
       ta.subject_id, s.code AS subject_code, s.name AS subject_name,
       ta.class_id, c.name AS class_name
FROM attendance_sessions ses
JOIN teaching_assignments ta ON ta.id = ses.teaching_assignment_id
JOIN teachers t ON t.id = ta.teacher_id
JOIN subjects s ON s.id = ta.subject_id
JOIN classes c ON c.id = ta.class_id
WHERE ses.session_date >= sqlc.arg(from_date)::date
  AND ses.session_date <= sqlc.arg(to_date)::date
  AND (sqlc.narg(class_id)::uuid IS NULL OR ta.class_id = sqlc.narg(class_id)::uuid)
  AND (sqlc.narg(subject_id)::uuid IS NULL OR ta.subject_id = sqlc.narg(subject_id)::uuid)
  AND (sqlc.narg(teacher_id)::uuid IS NULL OR ta.teacher_id = sqlc.narg(teacher_id)::uuid)
  AND (sqlc.narg(status)::text IS NULL OR ses.status = sqlc.narg(status)::text)
ORDER BY ses.session_date DESC, ses.period_start, ses.id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountAttendanceSessions :one
SELECT count(*)
FROM attendance_sessions ses
JOIN teaching_assignments ta ON ta.id = ses.teaching_assignment_id
WHERE ses.session_date >= sqlc.arg(from_date)::date
  AND ses.session_date <= sqlc.arg(to_date)::date
  AND (sqlc.narg(class_id)::uuid IS NULL OR ta.class_id = sqlc.narg(class_id)::uuid)
  AND (sqlc.narg(subject_id)::uuid IS NULL OR ta.subject_id = sqlc.narg(subject_id)::uuid)
  AND (sqlc.narg(teacher_id)::uuid IS NULL OR ta.teacher_id = sqlc.narg(teacher_id)::uuid)
  AND (sqlc.narg(status)::text IS NULL OR ses.status = sqlc.narg(status)::text);

-- name: CloseAttendanceSession :one
UPDATE attendance_sessions
SET status     = 'closed',
    closed_by  = sqlc.arg(closed_by),
    closed_at  = now(),
    updated_at = now()
WHERE id = sqlc.arg(id) AND status = 'open'
RETURNING id, teaching_assignment_id, session_date, period_start, period_end, topic,
          notes, status, created_by, closed_by, closed_at, created_at, updated_at;

-- name: ListSessionRoster :many
SELECT e.id AS enrollment_id, e.student_id, st.student_number, st.full_name,
       r.id AS record_id, r.status, r.note
FROM attendance_sessions ses
JOIN teaching_assignments ta ON ta.id = ses.teaching_assignment_id
JOIN class_enrollments e
  ON e.class_id = ta.class_id
 AND e.start_date <= ses.session_date
 AND (e.end_date IS NULL OR e.end_date >= ses.session_date)
JOIN students st ON st.id = e.student_id
LEFT JOIN attendance_session_records r
  ON r.session_id = ses.id AND r.student_id = e.student_id
WHERE ses.id = sqlc.arg(session_id)
ORDER BY st.full_name, st.id;

-- name: ListSessionRecords :many
SELECT r.id, r.session_id, r.student_id, st.student_number, st.full_name,
       r.enrollment_id, r.status, r.note, r.recorded_by, r.updated_by,
       r.created_at, r.updated_at
FROM attendance_session_records r
JOIN students st ON st.id = r.student_id
WHERE r.session_id = sqlc.arg(session_id)
ORDER BY st.full_name, st.id;

-- name: UpsertSessionRecord :one
INSERT INTO attendance_session_records
  (session_id, student_id, enrollment_id, status, note, recorded_by)
VALUES
  (sqlc.arg(session_id), sqlc.arg(student_id), sqlc.arg(enrollment_id),
   sqlc.arg(status), sqlc.narg(note), sqlc.arg(recorded_by))
ON CONFLICT (session_id, student_id) DO UPDATE
SET enrollment_id = EXCLUDED.enrollment_id,
    status        = EXCLUDED.status,
    note          = EXCLUDED.note,
    updated_by    = EXCLUDED.recorded_by,
    updated_at    = now()
RETURNING id, session_id, student_id, enrollment_id, status, note, recorded_by,
          updated_by, created_at, updated_at;

-- name: SummarizeAttendanceSessions :many
SELECT ta.id AS teaching_assignment_id,
       ta.class_id, c.name AS class_name,
       ta.subject_id, s.code AS subject_code, s.name AS subject_name,
       ta.teacher_id, t.full_name AS teacher_name,
       count(DISTINCT ses.id) AS sessions,
       count(r.id) AS records,
       COALESCE(sum(CASE WHEN r.status = 'present' THEN 1 ELSE 0 END), 0)::bigint AS present,
       COALESCE(sum(CASE WHEN r.status = 'late' THEN 1 ELSE 0 END), 0)::bigint AS late,
       COALESCE(sum(CASE WHEN r.status = 'sick' THEN 1 ELSE 0 END), 0)::bigint AS sick,
       COALESCE(sum(CASE WHEN r.status = 'excused' THEN 1 ELSE 0 END), 0)::bigint AS excused,
       COALESCE(sum(CASE WHEN r.status = 'absent' THEN 1 ELSE 0 END), 0)::bigint AS absent
FROM attendance_sessions ses
JOIN teaching_assignments ta ON ta.id = ses.teaching_assignment_id
JOIN classes c ON c.id = ta.class_id
JOIN subjects s ON s.id = ta.subject_id
JOIN teachers t ON t.id = ta.teacher_id
LEFT JOIN attendance_session_records r ON r.session_id = ses.id
WHERE ses.session_date >= sqlc.arg(from_date)::date
  AND ses.session_date <= sqlc.arg(to_date)::date
  AND (sqlc.narg(class_id)::uuid IS NULL OR ta.class_id = sqlc.narg(class_id)::uuid)
  AND (sqlc.narg(subject_id)::uuid IS NULL OR ta.subject_id = sqlc.narg(subject_id)::uuid)
  AND (sqlc.narg(teacher_id)::uuid IS NULL OR ta.teacher_id = sqlc.narg(teacher_id)::uuid)
GROUP BY ta.id, ta.class_id, c.name, ta.subject_id, s.code, s.name, ta.teacher_id, t.full_name
ORDER BY c.name, s.name, t.full_name, ta.id;
