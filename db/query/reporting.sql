-- name: DashboardAttendanceSummary :one
SELECT count(*)::int AS total_students,
       count(ar.id)::int AS recorded,
       (count(*) - count(ar.id))::int AS unrecorded,
       count(*) FILTER (WHERE ar.status = 'present')::int AS present,
       count(*) FILTER (WHERE ar.status = 'late')::int AS late,
       count(*) FILTER (WHERE ar.status = 'sick')::int AS sick,
       count(*) FILTER (WHERE ar.status = 'excused')::int AS excused,
       count(*) FILTER (WHERE ar.status = 'absent')::int AS absent
FROM class_enrollments e
JOIN students s ON s.id = e.student_id
JOIN classes c ON c.id = e.class_id
LEFT JOIN attendance_records ar
  ON ar.student_id = s.id AND ar.attendance_date = sqlc.arg(attendance_date)::date
WHERE s.status = 'active'
  AND e.start_date <= sqlc.arg(attendance_date)::date
  AND (e.end_date IS NULL OR e.end_date >= sqlc.arg(attendance_date)::date)
  AND (sqlc.narg(class_id)::uuid IS NULL OR e.class_id = sqlc.narg(class_id))
  AND (
    sqlc.arg(allow_all)::boolean
    OR (sqlc.arg(is_teacher)::boolean AND EXISTS (
      SELECT 1 FROM homeroom_assignments ha
      WHERE ha.class_id = e.class_id
        AND ha.teacher_id = sqlc.arg(teacher_id)::uuid
        AND ha.start_date <= sqlc.arg(attendance_date)::date
        AND (ha.end_date IS NULL OR ha.end_date >= sqlc.arg(attendance_date)::date)
    ))
    OR (sqlc.arg(is_duty)::boolean AND EXISTS (
      SELECT 1 FROM duty_assignments da
      WHERE da.teacher_id = sqlc.arg(teacher_id)::uuid
        AND da.weekday = EXTRACT(ISODOW FROM sqlc.arg(attendance_date)::date)::smallint
        AND da.start_date <= sqlc.arg(attendance_date)::date
        AND (da.end_date IS NULL OR da.end_date >= sqlc.arg(attendance_date)::date)
    ))
  );

-- name: DashboardAttendanceTrend :many
SELECT ar.attendance_date,
       count(*)::int AS recorded,
       count(*) FILTER (WHERE ar.status = 'present')::int AS present,
       count(*) FILTER (WHERE ar.status = 'late')::int AS late,
       count(*) FILTER (WHERE ar.status = 'sick')::int AS sick,
       count(*) FILTER (WHERE ar.status = 'excused')::int AS excused,
       count(*) FILTER (WHERE ar.status = 'absent')::int AS absent
FROM attendance_records ar
JOIN class_enrollments e ON e.id = ar.enrollment_id
WHERE ar.attendance_date BETWEEN sqlc.arg(from_date)::date AND sqlc.arg(to_date)::date
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
GROUP BY ar.attendance_date
ORDER BY ar.attendance_date;

-- name: DashboardLateStudents :many
SELECT ar.student_id, s.student_number, s.full_name,
       e.class_id, c.name AS class_name, ar.check_in_at
FROM attendance_records ar
JOIN students s ON s.id = ar.student_id
JOIN class_enrollments e ON e.id = ar.enrollment_id
JOIN classes c ON c.id = e.class_id
WHERE ar.attendance_date = sqlc.arg(attendance_date)::date
  AND ar.status = 'late'
  AND (sqlc.narg(class_id)::uuid IS NULL OR e.class_id = sqlc.narg(class_id))
  AND (
    sqlc.arg(allow_all)::boolean
    OR (sqlc.arg(is_teacher)::boolean AND EXISTS (
      SELECT 1 FROM homeroom_assignments ha
      WHERE ha.class_id = e.class_id
        AND ha.teacher_id = sqlc.arg(teacher_id)::uuid
        AND ha.start_date <= sqlc.arg(attendance_date)::date
        AND (ha.end_date IS NULL OR ha.end_date >= sqlc.arg(attendance_date)::date)
    ))
    OR (sqlc.arg(is_duty)::boolean AND EXISTS (
      SELECT 1 FROM duty_assignments da
      WHERE da.teacher_id = sqlc.arg(teacher_id)::uuid
        AND da.weekday = EXTRACT(ISODOW FROM sqlc.arg(attendance_date)::date)::smallint
        AND da.start_date <= sqlc.arg(attendance_date)::date
        AND (da.end_date IS NULL OR da.end_date >= sqlc.arg(attendance_date)::date)
    ))
  )
ORDER BY ar.check_in_at NULLS LAST, s.full_name
LIMIT 50;

-- name: DashboardUnrecordedStudents :many
SELECT s.id AS student_id, s.student_number, s.full_name,
       e.class_id, c.name AS class_name
FROM class_enrollments e
JOIN students s ON s.id = e.student_id
JOIN classes c ON c.id = e.class_id
WHERE s.status = 'active'
  AND e.start_date <= sqlc.arg(attendance_date)::date
  AND (e.end_date IS NULL OR e.end_date >= sqlc.arg(attendance_date)::date)
  AND (sqlc.narg(class_id)::uuid IS NULL OR e.class_id = sqlc.narg(class_id))
  AND NOT EXISTS (
    SELECT 1 FROM attendance_records ar
    WHERE ar.student_id = s.id AND ar.attendance_date = sqlc.arg(attendance_date)::date
  )
  AND (
    sqlc.arg(allow_all)::boolean
    OR (sqlc.arg(is_teacher)::boolean AND EXISTS (
      SELECT 1 FROM homeroom_assignments ha
      WHERE ha.class_id = e.class_id
        AND ha.teacher_id = sqlc.arg(teacher_id)::uuid
        AND ha.start_date <= sqlc.arg(attendance_date)::date
        AND (ha.end_date IS NULL OR ha.end_date >= sqlc.arg(attendance_date)::date)
    ))
    OR (sqlc.arg(is_duty)::boolean AND EXISTS (
      SELECT 1 FROM duty_assignments da
      WHERE da.teacher_id = sqlc.arg(teacher_id)::uuid
        AND da.weekday = EXTRACT(ISODOW FROM sqlc.arg(attendance_date)::date)::smallint
        AND da.start_date <= sqlc.arg(attendance_date)::date
        AND (da.end_date IS NULL OR da.end_date >= sqlc.arg(attendance_date)::date)
    ))
  )
ORDER BY c.name, s.full_name
LIMIT 50;

-- name: DashboardDisciplineSummary :one
SELECT count(*) FILTER (
         WHERE school_local_date(dr.occurred_at) = sqlc.arg(attendance_date)::date
       )::int AS records_today,
       count(*)::int AS records_month,
       COALESCE(sum(dr.points) FILTER (WHERE dr.status <> 'cancelled'), 0)::int AS points_month,
       count(*) FILTER (WHERE dr.status = 'new')::int AS new_records
FROM discipline_records dr
JOIN class_enrollments e ON e.id = dr.enrollment_id
WHERE dr.occurred_at >= sqlc.arg(month_start_at)::timestamptz
  AND dr.occurred_at < sqlc.arg(month_end_at)::timestamptz
  AND (sqlc.narg(class_id)::uuid IS NULL OR e.class_id = sqlc.narg(class_id))
  AND (
    sqlc.arg(allow_all)::boolean
    OR (sqlc.arg(is_teacher)::boolean AND EXISTS (
      SELECT 1 FROM homeroom_assignments ha
      WHERE ha.class_id = e.class_id
        AND ha.teacher_id = sqlc.arg(teacher_id)::uuid
        AND ha.start_date <= school_local_date(dr.occurred_at)
        AND (ha.end_date IS NULL OR ha.end_date >= school_local_date(dr.occurred_at))
    ))
    OR (sqlc.arg(is_duty)::boolean AND EXISTS (
      SELECT 1 FROM duty_assignments da
      WHERE da.teacher_id = sqlc.arg(teacher_id)::uuid
        AND da.weekday = EXTRACT(ISODOW FROM school_local_date(dr.occurred_at))::smallint
        AND da.start_date <= school_local_date(dr.occurred_at)
        AND (da.end_date IS NULL OR da.end_date >= school_local_date(dr.occurred_at))
    ))
  );

-- name: DashboardSystemCounts :one
SELECT (SELECT count(*)::int FROM students WHERE status = 'active') AS active_students,
       (SELECT count(*)::int FROM teachers WHERE status = 'active') AS active_teachers;

-- name: ListAuditLogs :many
SELECT al.id,
       al.actor_user_id,
       COALESCE(u.full_name, '') AS actor_name,
       al.action,
       al.entity_type,
       al.entity_id,
       COALESCE(al.request_id, '') AS request_id,
       COALESCE(al.before_data::text, '') AS before_data,
       COALESCE(al.after_data::text, '') AS after_data,
       COALESCE(al.reason, '') AS reason,
       al.created_at
FROM audit_logs al
LEFT JOIN users u ON u.id = al.actor_user_id
WHERE (sqlc.narg(action)::text IS NULL OR al.action = sqlc.narg(action))
  AND (sqlc.narg(entity_type)::text IS NULL OR al.entity_type = sqlc.narg(entity_type))
  AND (sqlc.narg(entity_id)::uuid IS NULL OR al.entity_id = sqlc.narg(entity_id))
  AND (sqlc.narg(actor_id)::uuid IS NULL OR al.actor_user_id = sqlc.narg(actor_id))
  AND (sqlc.narg(from_at)::timestamptz IS NULL OR al.created_at >= sqlc.narg(from_at))
  AND (sqlc.narg(to_at)::timestamptz IS NULL OR al.created_at < sqlc.narg(to_at))
ORDER BY al.created_at DESC, al.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountAuditLogs :one
SELECT count(*)
FROM audit_logs al
WHERE (sqlc.narg(action)::text IS NULL OR al.action = sqlc.narg(action))
  AND (sqlc.narg(entity_type)::text IS NULL OR al.entity_type = sqlc.narg(entity_type))
  AND (sqlc.narg(entity_id)::uuid IS NULL OR al.entity_id = sqlc.narg(entity_id))
  AND (sqlc.narg(actor_id)::uuid IS NULL OR al.actor_user_id = sqlc.narg(actor_id))
  AND (sqlc.narg(from_at)::timestamptz IS NULL OR al.created_at >= sqlc.narg(from_at))
  AND (sqlc.narg(to_at)::timestamptz IS NULL OR al.created_at < sqlc.narg(to_at));

-- name: ListAttendanceReport :many
SELECT date_trunc(sqlc.arg(period)::text, ar.attendance_date)::date AS period_start,
       c.id AS class_id,
       c.name AS class_name,
       count(*)::int AS total_days,
       count(*) FILTER (WHERE ar.status = 'present')::int AS present,
       count(*) FILTER (WHERE ar.status = 'late')::int AS late,
       count(*) FILTER (WHERE ar.status = 'sick')::int AS sick,
       count(*) FILTER (WHERE ar.status = 'excused')::int AS excused,
       count(*) FILTER (WHERE ar.status = 'absent')::int AS absent
FROM attendance_records ar
JOIN class_enrollments e ON e.id = ar.enrollment_id
JOIN classes c ON c.id = e.class_id
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
GROUP BY period_start, c.id, c.name
ORDER BY period_start, c.name;
