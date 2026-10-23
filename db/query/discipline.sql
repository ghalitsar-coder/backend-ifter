-- name: ListDisciplineCategories :many
SELECT id, code, name, description, default_points, auto_on_attendance_status,
       is_active, created_at, updated_at
FROM discipline_categories
WHERE (sqlc.narg(is_active)::boolean IS NULL OR is_active = sqlc.narg(is_active))
ORDER BY is_active DESC, name, id;

-- name: GetDisciplineCategory :one
SELECT id, code, name, description, default_points, auto_on_attendance_status,
       is_active, created_at, updated_at
FROM discipline_categories
WHERE id = sqlc.arg(id);

-- name: GetDisciplineCategoryForUpdate :one
SELECT id, code, name, description, default_points, auto_on_attendance_status,
       is_active, created_at, updated_at
FROM discipline_categories
WHERE id = sqlc.arg(id)
FOR UPDATE;

-- name: CreateDisciplineCategory :one
INSERT INTO discipline_categories (
  code, name, description, default_points, auto_on_attendance_status, is_active
)
VALUES (
  sqlc.narg(code), sqlc.arg(name), sqlc.narg(description),
  sqlc.narg(default_points), sqlc.narg(auto_on_attendance_status), true
)
RETURNING id, code, name, description, default_points, auto_on_attendance_status,
          is_active, created_at, updated_at;

-- name: UpdateDisciplineCategory :one
UPDATE discipline_categories
SET code = sqlc.narg(code),
    name = sqlc.arg(name),
    description = sqlc.narg(description),
    default_points = sqlc.narg(default_points),
    auto_on_attendance_status = sqlc.narg(auto_on_attendance_status),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id, code, name, description, default_points, auto_on_attendance_status,
          is_active, created_at, updated_at;

-- name: SetDisciplineCategoryStatus :one
UPDATE discipline_categories
SET is_active = sqlc.arg(is_active), updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id, code, name, description, default_points, auto_on_attendance_status,
          is_active, created_at, updated_at;

-- name: GetStudentEnrollmentAtDate :one
SELECT e.id, e.student_id, e.class_id, e.start_date, e.end_date,
       s.student_number, s.full_name AS student_name, c.name AS class_name
FROM class_enrollments e
JOIN students s ON s.id = e.student_id
JOIN classes c ON c.id = e.class_id
WHERE e.student_id = sqlc.arg(student_id)
  AND school_local_date(sqlc.arg(occurred_at)::timestamptz)
      BETWEEN e.start_date AND COALESCE(e.end_date, 'infinity'::date)
ORDER BY e.start_date DESC, e.id DESC
LIMIT 1;

-- name: GetCurrentStudentEnrollment :one
SELECT e.id, e.student_id, e.class_id, e.start_date, e.end_date,
       s.student_number, s.full_name AS student_name, c.name AS class_name
FROM class_enrollments e
JOIN students s ON s.id = e.student_id
JOIN classes c ON c.id = e.class_id
WHERE e.student_id = sqlc.arg(student_id)
  AND e.start_date <= school_local_date(now())
  AND (e.end_date IS NULL OR e.end_date >= school_local_date(now()))
ORDER BY e.start_date DESC, e.id DESC
LIMIT 1;

-- name: CreateDisciplineRecord :one
INSERT INTO discipline_records (
  student_id, enrollment_id, category_id, occurred_at, description,
  points, status, recorded_by_user_id
)
VALUES (
  sqlc.arg(student_id), sqlc.arg(enrollment_id), sqlc.arg(category_id),
  sqlc.arg(occurred_at)::timestamptz, sqlc.arg(description),
  sqlc.arg(points), 'new', sqlc.arg(recorded_by_user_id)
)
RETURNING id;

-- name: GetDisciplineRecord :one
SELECT dr.id, dr.student_id, s.student_number, s.full_name AS student_name,
       e.class_id, c.name AS class_name, dr.category_id, dc.name AS category_name,
       dr.occurred_at, school_local_date(dr.occurred_at) AS occurred_date,
       dr.description, dr.points, dr.status, dr.attendance_record_id,
       dr.recorded_by_user_id, u.full_name AS recorded_by_name,
       dr.created_at, dr.updated_at
FROM discipline_records dr
JOIN students s ON s.id = dr.student_id
JOIN class_enrollments e ON e.id = dr.enrollment_id
JOIN classes c ON c.id = e.class_id
JOIN discipline_categories dc ON dc.id = dr.category_id
JOIN users u ON u.id = dr.recorded_by_user_id
WHERE dr.id = sqlc.arg(id);

-- name: GetDisciplineRecordForUpdate :one
SELECT dr.id, dr.student_id, s.student_number, s.full_name AS student_name,
       e.class_id, c.name AS class_name, dr.category_id, dc.name AS category_name,
       dr.occurred_at, school_local_date(dr.occurred_at) AS occurred_date,
       dr.description, dr.points, dr.status, dr.attendance_record_id,
       dr.recorded_by_user_id, u.full_name AS recorded_by_name,
       dr.created_at, dr.updated_at
FROM discipline_records dr
JOIN students s ON s.id = dr.student_id
JOIN class_enrollments e ON e.id = dr.enrollment_id
JOIN classes c ON c.id = e.class_id
JOIN discipline_categories dc ON dc.id = dr.category_id
JOIN users u ON u.id = dr.recorded_by_user_id
WHERE dr.id = sqlc.arg(id)
FOR UPDATE OF dr;

-- name: ListDisciplineRecords :many
SELECT dr.id, dr.student_id, s.student_number, s.full_name AS student_name,
       e.class_id, c.name AS class_name, dr.category_id, dc.name AS category_name,
       dr.occurred_at, school_local_date(dr.occurred_at) AS occurred_date,
       dr.description, dr.points, dr.status, dr.attendance_record_id,
       dr.recorded_by_user_id, u.full_name AS recorded_by_name,
       dr.created_at, dr.updated_at
FROM discipline_records dr
JOIN students s ON s.id = dr.student_id
JOIN class_enrollments e ON e.id = dr.enrollment_id
JOIN classes c ON c.id = e.class_id
JOIN discipline_categories dc ON dc.id = dr.category_id
JOIN users u ON u.id = dr.recorded_by_user_id
WHERE (sqlc.narg(student_id)::uuid IS NULL OR dr.student_id = sqlc.narg(student_id))
  AND (sqlc.narg(class_id)::uuid IS NULL OR e.class_id = sqlc.narg(class_id))
  AND (sqlc.narg(category_id)::uuid IS NULL OR dr.category_id = sqlc.narg(category_id))
  AND (sqlc.narg(recorded_by)::uuid IS NULL OR dr.recorded_by_user_id = sqlc.narg(recorded_by))
  AND (sqlc.narg(status)::text IS NULL OR dr.status = sqlc.narg(status))
  AND (sqlc.narg(from_date)::date IS NULL OR school_local_date(dr.occurred_at) >= sqlc.narg(from_date))
  AND (sqlc.narg(to_date)::date IS NULL OR school_local_date(dr.occurred_at) <= sqlc.narg(to_date))
  AND (
    sqlc.arg(allow_all)::boolean
    OR (sqlc.arg(is_teacher)::boolean AND EXISTS (
      SELECT 1 FROM homeroom_assignments ha
      WHERE ha.class_id = e.class_id
        AND ha.teacher_id = sqlc.arg(teacher_id)
        AND school_local_date(dr.occurred_at)
            BETWEEN ha.start_date AND COALESCE(ha.end_date, 'infinity'::date)
    ))
    OR (sqlc.arg(is_duty)::boolean AND EXISTS (
      SELECT 1 FROM duty_assignments da
      WHERE da.teacher_id = sqlc.arg(teacher_id)
        AND da.weekday = EXTRACT(ISODOW FROM school_local_date(dr.occurred_at))::smallint
        AND school_local_date(dr.occurred_at)
            BETWEEN da.start_date AND COALESCE(da.end_date, 'infinity'::date)
    ))
  )
ORDER BY dr.occurred_at DESC, dr.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountDisciplineRecords :one
SELECT count(*)
FROM discipline_records dr
JOIN class_enrollments e ON e.id = dr.enrollment_id
WHERE (sqlc.narg(student_id)::uuid IS NULL OR dr.student_id = sqlc.narg(student_id))
  AND (sqlc.narg(class_id)::uuid IS NULL OR e.class_id = sqlc.narg(class_id))
  AND (sqlc.narg(category_id)::uuid IS NULL OR dr.category_id = sqlc.narg(category_id))
  AND (sqlc.narg(recorded_by)::uuid IS NULL OR dr.recorded_by_user_id = sqlc.narg(recorded_by))
  AND (sqlc.narg(status)::text IS NULL OR dr.status = sqlc.narg(status))
  AND (sqlc.narg(from_date)::date IS NULL OR school_local_date(dr.occurred_at) >= sqlc.narg(from_date))
  AND (sqlc.narg(to_date)::date IS NULL OR school_local_date(dr.occurred_at) <= sqlc.narg(to_date))
  AND (
    sqlc.arg(allow_all)::boolean
    OR (sqlc.arg(is_teacher)::boolean AND EXISTS (
      SELECT 1 FROM homeroom_assignments ha
      WHERE ha.class_id = e.class_id
        AND ha.teacher_id = sqlc.arg(teacher_id)
        AND school_local_date(dr.occurred_at)
            BETWEEN ha.start_date AND COALESCE(ha.end_date, 'infinity'::date)
    ))
    OR (sqlc.arg(is_duty)::boolean AND EXISTS (
      SELECT 1 FROM duty_assignments da
      WHERE da.teacher_id = sqlc.arg(teacher_id)
        AND da.weekday = EXTRACT(ISODOW FROM school_local_date(dr.occurred_at))::smallint
        AND school_local_date(dr.occurred_at)
            BETWEEN da.start_date AND COALESCE(da.end_date, 'infinity'::date)
    ))
  );

-- name: ListDisciplineFollowups :many
SELECT f.id, f.discipline_record_id, f.status, f.notes,
       f.updated_by_user_id, u.full_name AS updated_by_name, f.created_at
FROM discipline_followups f
JOIN users u ON u.id = f.updated_by_user_id
WHERE f.discipline_record_id = sqlc.arg(discipline_record_id)
ORDER BY f.created_at, f.id;

-- name: CreateDisciplineFollowup :one
INSERT INTO discipline_followups (discipline_record_id, status, notes, updated_by_user_id)
VALUES (
  sqlc.arg(discipline_record_id), sqlc.arg(status),
  sqlc.narg(notes), sqlc.arg(updated_by_user_id)
)
RETURNING id;

-- name: UpdateDisciplineRecordStatus :one
UPDATE discipline_records
SET status = sqlc.arg(status), updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id;

-- name: UpdateDisciplineRecordPoints :one
UPDATE discipline_records
SET points = sqlc.arg(points), updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id;
