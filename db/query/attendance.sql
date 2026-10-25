-- name: GetSchoolSettings :one
SELECT id, school_name, timezone, attendance_auto_close_time, updated_by, updated_at
FROM school_settings
WHERE id = 1;

-- name: UpdateSchoolSettings :one
UPDATE school_settings
SET school_name = sqlc.arg(school_name),
    timezone = sqlc.arg(timezone),
    attendance_auto_close_time = sqlc.arg(attendance_auto_close_time),
    updated_by = sqlc.arg(updated_by),
    updated_at = now()
WHERE id = 1
RETURNING id, school_name, timezone, attendance_auto_close_time, updated_by, updated_at;

-- name: ListSchoolSchedules :many
SELECT weekday, start_time, late_tolerance_minutes
FROM school_schedules
ORDER BY weekday;

-- name: DeleteSchoolSchedules :exec
DELETE FROM school_schedules;

-- name: UpsertSchoolSchedule :exec
INSERT INTO school_schedules (weekday, start_time, late_tolerance_minutes)
VALUES (sqlc.arg(weekday), sqlc.arg(start_time), sqlc.arg(late_tolerance_minutes))
ON CONFLICT (weekday) DO UPDATE
SET start_time = EXCLUDED.start_time,
    late_tolerance_minutes = EXCLUDED.late_tolerance_minutes;

-- name: ListSchoolHolidays :many
SELECT holiday_date, name, created_by, created_at
FROM school_holidays
WHERE (sqlc.narg(from_date)::date IS NULL OR holiday_date >= sqlc.narg(from_date))
  AND (sqlc.narg(to_date)::date IS NULL OR holiday_date <= sqlc.narg(to_date))
ORDER BY holiday_date;

-- name: CreateSchoolHoliday :one
INSERT INTO school_holidays (holiday_date, name, created_by)
VALUES (sqlc.arg(holiday_date), sqlc.arg(name), sqlc.arg(created_by))
RETURNING holiday_date, name, created_by, created_at;

-- name: DeleteSchoolHoliday :execrows
DELETE FROM school_holidays WHERE holiday_date = sqlc.arg(holiday_date);

-- name: GetClassDayContext :one
SELECT c.id, c.name, c.status,
       ay.id AS academic_year_id, ay.name AS academic_year_name,
       ay.start_date AS academic_year_start, ay.end_date AS academic_year_end,
       ay.is_active AS academic_year_active,
       attendance_day_is_effective(c.id, sqlc.arg(attendance_date)::date) AS effective,
       (ss.weekday IS NOT NULL)::boolean AS has_schedule,
       ss.start_time, ss.late_tolerance_minutes,
       EXISTS (SELECT 1 FROM school_holidays h
               WHERE h.holiday_date = sqlc.arg(attendance_date)::date) AS is_holiday
FROM classes c
JOIN academic_years ay ON ay.id = c.academic_year_id
LEFT JOIN school_schedules ss
  ON ss.weekday = EXTRACT(ISODOW FROM sqlc.arg(attendance_date)::date)::smallint
WHERE c.id = sqlc.arg(class_id);

-- name: GetTeacherByUserID :one
SELECT id, user_id, employee_number, full_name, email, phone, status, created_at, updated_at, employment_type_id
FROM teachers
WHERE user_id = sqlc.arg(user_id);

-- name: HasHomeroomAssignmentForDate :one
SELECT EXISTS (
  SELECT 1 FROM homeroom_assignments
  WHERE class_id = sqlc.arg(class_id)
    AND teacher_id = sqlc.arg(teacher_id)
    AND start_date <= sqlc.arg(attendance_date)::date
    AND (end_date IS NULL OR end_date >= sqlc.arg(attendance_date)::date)
);

-- name: HasDutyAssignmentForDate :one
SELECT EXISTS (
  SELECT 1 FROM duty_assignments
  WHERE teacher_id = sqlc.arg(teacher_id)
    AND weekday = EXTRACT(ISODOW FROM sqlc.arg(attendance_date)::date)::smallint
    AND start_date <= sqlc.arg(attendance_date)::date
    AND (end_date IS NULL OR end_date >= sqlc.arg(attendance_date)::date)
);

-- name: ListAttendanceRoster :many
SELECT e.id AS enrollment_id, s.id AS student_id, s.student_number, s.full_name,
       ar.id AS attendance_id, ar.status, ar.check_in_at, ar.notes, ar.source,
       ar.recorded_by_user_id, ar.updated_by_user_id, ar.created_at, ar.updated_at
FROM class_enrollments e
JOIN students s ON s.id = e.student_id
LEFT JOIN attendance_records ar
  ON ar.student_id = s.id AND ar.attendance_date = sqlc.arg(attendance_date)::date
WHERE e.class_id = sqlc.arg(class_id)
  AND e.start_date <= sqlc.arg(attendance_date)::date
  AND (e.end_date IS NULL OR e.end_date >= sqlc.arg(attendance_date)::date)
ORDER BY s.full_name, s.id;

-- name: GetAttendanceClosure :one
SELECT cl.id, cl.class_id, cl.attendance_date, cl.close_type,
       cl.closed_by_user_id, u.full_name AS closed_by_name,
       cl.closed_at, cl.absent_filled
FROM attendance_day_closures cl
JOIN users u ON u.id = cl.closed_by_user_id
WHERE cl.class_id = sqlc.arg(class_id)
  AND cl.attendance_date = sqlc.arg(attendance_date)::date;

-- name: CreateAttendanceClosure :one
INSERT INTO attendance_day_closures (class_id, attendance_date, close_type, closed_by_user_id)
VALUES (sqlc.arg(class_id), sqlc.arg(attendance_date)::date, sqlc.arg(close_type), sqlc.arg(closed_by_user_id))
RETURNING id, class_id, attendance_date, close_type, closed_by_user_id, closed_at, absent_filled;

-- name: UpdateAttendanceClosureFilled :exec
UPDATE attendance_day_closures
SET absent_filled = sqlc.arg(absent_filled)
WHERE id = sqlc.arg(id);

-- name: GetSystemUserID :one
SELECT id FROM users WHERE username = 'system' AND status = 'system';

-- name: FillMissingAttendanceAbsent :execrows
INSERT INTO attendance_records (
  student_id, enrollment_id, attendance_date, status, source,
  recorded_by_user_id, updated_by_user_id
)
SELECT e.student_id, e.id, sqlc.arg(attendance_date)::date, 'absent', 'system_close',
       sqlc.arg(system_user_id), sqlc.arg(system_user_id)
FROM class_enrollments e
WHERE e.class_id = sqlc.arg(class_id)
  AND e.start_date <= sqlc.arg(attendance_date)::date
  AND (e.end_date IS NULL OR e.end_date >= sqlc.arg(attendance_date)::date)
  AND NOT EXISTS (
    SELECT 1 FROM attendance_records ar
    WHERE ar.student_id = e.student_id
      AND ar.attendance_date = sqlc.arg(attendance_date)::date
  );

-- name: UpsertAttendanceRecord :one
INSERT INTO attendance_records (
  student_id, enrollment_id, attendance_date, status, check_in_at, notes, source,
  recorded_by_user_id, updated_by_user_id
)
VALUES (
  sqlc.arg(student_id), sqlc.arg(enrollment_id), sqlc.arg(attendance_date)::date,
  sqlc.arg(status), sqlc.narg(check_in_at), sqlc.narg(notes), 'manual',
  sqlc.arg(actor_user_id), sqlc.arg(actor_user_id)
)
ON CONFLICT (student_id, attendance_date) DO UPDATE
SET status = EXCLUDED.status,
    check_in_at = EXCLUDED.check_in_at,
    notes = EXCLUDED.notes,
    source = 'manual',
    updated_by_user_id = EXCLUDED.updated_by_user_id,
    updated_at = now()
WHERE attendance_records.enrollment_id = EXCLUDED.enrollment_id
RETURNING id, student_id, enrollment_id, attendance_date, status, check_in_at, notes,
          source, recorded_by_user_id, updated_by_user_id, created_at, updated_at;

-- name: GetAttendanceRecordForUpdate :one
SELECT ar.id, ar.student_id, ar.enrollment_id, ar.attendance_date, ar.status,
       ar.check_in_at, ar.notes, ar.source, ar.recorded_by_user_id,
       ar.updated_by_user_id, ar.created_at, ar.updated_at, e.class_id
FROM attendance_records ar
JOIN class_enrollments e ON e.id = ar.enrollment_id
WHERE ar.id = sqlc.arg(id)
FOR UPDATE OF ar;

-- name: GetAttendanceRecord :one
SELECT ar.id, ar.student_id, ar.enrollment_id, ar.attendance_date, ar.status,
       ar.check_in_at, ar.notes, ar.source, ar.recorded_by_user_id,
       ar.updated_by_user_id, ar.created_at, ar.updated_at,
       e.class_id, c.name AS class_name, s.student_number, s.full_name
FROM attendance_records ar
JOIN class_enrollments e ON e.id = ar.enrollment_id
JOIN classes c ON c.id = e.class_id
JOIN students s ON s.id = ar.student_id
WHERE ar.id = sqlc.arg(id);

-- name: CreateAttendanceCorrection :one
-- evidence_hash + corrected_at are computed server-side by the
-- correction-evidence package (canonical sha256, docs/API_ADDENDUM.md §6.6);
-- corrected_at is passed explicitly so the hash and the stored row cover the
-- exact same instant.
INSERT INTO attendance_corrections (
  attendance_record_id, old_status, new_status, old_check_in_at, new_check_in_at,
  old_notes, new_notes, reason, corrected_by_user_id, evidence_hash, corrected_at
)
VALUES (
  sqlc.arg(attendance_record_id), sqlc.arg(old_status), sqlc.arg(new_status),
  sqlc.narg(old_check_in_at), sqlc.narg(new_check_in_at),
  sqlc.narg(old_notes), sqlc.narg(new_notes), sqlc.arg(reason),
  sqlc.arg(corrected_by_user_id), sqlc.narg(evidence_hash), sqlc.arg(corrected_at)
)
RETURNING id, attendance_record_id, old_status, new_status, old_check_in_at,
          new_check_in_at, old_notes, new_notes, reason, corrected_by_user_id,
          corrected_at, evidence_hash;

-- name: UpdateAttendanceRecord :one
UPDATE attendance_records
SET status = sqlc.arg(status),
    check_in_at = sqlc.narg(check_in_at),
    notes = sqlc.narg(notes),
    updated_by_user_id = sqlc.arg(actor_user_id),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id, student_id, enrollment_id, attendance_date, status, check_in_at, notes,
          source, recorded_by_user_id, updated_by_user_id, created_at, updated_at;

-- name: ListAttendanceCorrections :many
SELECT id, attendance_record_id, old_status, new_status, old_check_in_at,
       new_check_in_at, old_notes, new_notes, reason, corrected_by_user_id,
       corrected_at, evidence_hash
FROM attendance_corrections
WHERE attendance_record_id = sqlc.arg(attendance_record_id)
ORDER BY corrected_at DESC, id;

-- name: ListAttendanceHistory :many
SELECT ar.id, ar.attendance_date, ar.status, ar.check_in_at, ar.notes, ar.source,
       ar.created_at, ar.updated_at,
       s.id AS student_id, s.student_number, s.full_name,
       c.id AS class_id, c.name AS class_name,
       ay.id AS academic_year_id, ay.name AS academic_year_name,
       cl.id AS closure_id
FROM attendance_records ar
JOIN class_enrollments e ON e.id = ar.enrollment_id
JOIN students s ON s.id = ar.student_id
JOIN classes c ON c.id = e.class_id
JOIN academic_years ay ON ay.id = c.academic_year_id
LEFT JOIN attendance_day_closures cl
  ON cl.class_id = e.class_id AND cl.attendance_date = ar.attendance_date
WHERE ar.attendance_date BETWEEN sqlc.arg(from_date)::date AND sqlc.arg(to_date)::date
  AND (sqlc.narg(student_id)::uuid IS NULL OR ar.student_id = sqlc.narg(student_id))
  AND (sqlc.narg(class_id)::uuid IS NULL OR e.class_id = sqlc.narg(class_id))
  AND (sqlc.narg(status)::text IS NULL OR ar.status = sqlc.narg(status))
  AND (
    sqlc.arg(allow_all)::boolean
    OR (sqlc.arg(is_teacher)::boolean AND EXISTS (
      SELECT 1 FROM homeroom_assignments ha
      WHERE ha.class_id = e.class_id
        AND ha.teacher_id = sqlc.arg(teacher_id)::uuid
        AND ha.start_date <= ar.attendance_date
        AND (ha.end_date IS NULL OR ha.end_date >= ar.attendance_date)
    ))
    OR (sqlc.arg(is_duty)::boolean AND EXISTS (
      SELECT 1 FROM duty_assignments da
      WHERE da.teacher_id = sqlc.arg(teacher_id)::uuid
        AND da.weekday = EXTRACT(ISODOW FROM ar.attendance_date)::smallint
        AND da.start_date <= ar.attendance_date
        AND (da.end_date IS NULL OR da.end_date >= ar.attendance_date)
    ))
  )
ORDER BY ar.attendance_date DESC, s.full_name, ar.id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountAttendanceHistory :one
SELECT count(*)
FROM attendance_records ar
JOIN class_enrollments e ON e.id = ar.enrollment_id
WHERE ar.attendance_date BETWEEN sqlc.arg(from_date)::date AND sqlc.arg(to_date)::date
  AND (sqlc.narg(student_id)::uuid IS NULL OR ar.student_id = sqlc.narg(student_id))
  AND (sqlc.narg(class_id)::uuid IS NULL OR e.class_id = sqlc.narg(class_id))
  AND (sqlc.narg(status)::text IS NULL OR ar.status = sqlc.narg(status))
  AND (
    sqlc.arg(allow_all)::boolean
    OR (sqlc.arg(is_teacher)::boolean AND EXISTS (
      SELECT 1 FROM homeroom_assignments ha
      WHERE ha.class_id = e.class_id
        AND ha.teacher_id = sqlc.arg(teacher_id)::uuid
        AND ha.start_date <= ar.attendance_date
        AND (ha.end_date IS NULL OR ha.end_date >= ar.attendance_date)
    ))
    OR (sqlc.arg(is_duty)::boolean AND EXISTS (
      SELECT 1 FROM duty_assignments da
      WHERE da.teacher_id = sqlc.arg(teacher_id)::uuid
        AND da.weekday = EXTRACT(ISODOW FROM ar.attendance_date)::smallint
        AND da.start_date <= ar.attendance_date
        AND (da.end_date IS NULL OR da.end_date >= ar.attendance_date)
    ))
  );

-- name: ListAttendanceSummary :many
WITH scoped AS (
  SELECT ar.student_id, ar.status, e.class_id
  FROM attendance_records ar
  JOIN class_enrollments e ON e.id = ar.enrollment_id
  JOIN attendance_day_closures cl
    ON cl.class_id = e.class_id AND cl.attendance_date = ar.attendance_date
  WHERE ar.attendance_date BETWEEN sqlc.arg(from_date)::date AND sqlc.arg(to_date)::date
    AND (sqlc.narg(student_id)::uuid IS NULL OR ar.student_id = sqlc.narg(student_id))
    AND (sqlc.narg(class_id)::uuid IS NULL OR e.class_id = sqlc.narg(class_id))
    AND (
      sqlc.arg(allow_all)::boolean
      OR (sqlc.arg(is_teacher)::boolean AND EXISTS (
        SELECT 1 FROM homeroom_assignments ha
        WHERE ha.class_id = e.class_id
          AND ha.teacher_id = sqlc.arg(teacher_id)::uuid
          AND ha.start_date <= ar.attendance_date
          AND (ha.end_date IS NULL OR ha.end_date >= ar.attendance_date)
      ))
      OR (sqlc.arg(is_duty)::boolean AND EXISTS (
        SELECT 1 FROM duty_assignments da
        WHERE da.teacher_id = sqlc.arg(teacher_id)::uuid
          AND da.weekday = EXTRACT(ISODOW FROM ar.attendance_date)::smallint
          AND da.start_date <= ar.attendance_date
          AND (da.end_date IS NULL OR da.end_date >= ar.attendance_date)
      ))
    )
)
SELECT s.id AS student_id, s.student_number, s.full_name,
       c.id AS class_id, c.name AS class_name,
       count(*)::int AS total_days,
       count(*) FILTER (WHERE scoped.status = 'present')::int AS present_days,
       count(*) FILTER (WHERE scoped.status = 'late')::int AS late_days,
       count(*) FILTER (WHERE scoped.status = 'sick')::int AS sick_days,
       count(*) FILTER (WHERE scoped.status = 'excused')::int AS excused_days,
       count(*) FILTER (WHERE scoped.status = 'absent')::int AS absent_days
FROM scoped
JOIN students s ON s.id = scoped.student_id
JOIN classes c ON c.id = scoped.class_id
GROUP BY s.id, s.student_number, s.full_name, c.id, c.name
ORDER BY s.full_name, c.name;

-- name: ListClassesDueForAutoClose :many
SELECT c.id
FROM classes c
JOIN academic_years ay ON ay.id = c.academic_year_id
WHERE c.status = 'active'
  AND ay.is_active
  AND attendance_day_is_effective(c.id, sqlc.arg(attendance_date)::date)
  AND NOT EXISTS (
    SELECT 1 FROM attendance_day_closures cl
    WHERE cl.class_id = c.id
      AND cl.attendance_date = sqlc.arg(attendance_date)::date
  )
ORDER BY c.name;
