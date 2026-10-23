-- name: ListClasses :many
SELECT c.id, c.academic_year_id, ay.name AS academic_year_name, c.name,
       c.grade_level, c.major, c.status, c.created_at, c.updated_at,
       ht.id AS homeroom_teacher_id, COALESCE(ht.full_name, '') AS homeroom_teacher_name
FROM classes c
JOIN academic_years ay ON ay.id = c.academic_year_id
LEFT JOIN LATERAL (
  SELECT t.id, t.full_name
  FROM homeroom_assignments ha
  JOIN teachers t ON t.id = ha.teacher_id
  WHERE ha.class_id = c.id
    AND ha.start_date <= CURRENT_DATE
    AND (ha.end_date IS NULL OR ha.end_date >= CURRENT_DATE)
  ORDER BY ha.start_date DESC
  LIMIT 1
) ht ON true
WHERE (sqlc.narg(academic_year_id)::uuid IS NULL OR c.academic_year_id = sqlc.narg(academic_year_id))
  AND (sqlc.narg(status)::text IS NULL OR c.status = sqlc.narg(status))
ORDER BY ay.start_date DESC, c.grade_level, c.name
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountClasses :one
SELECT count(*)
FROM classes c
WHERE (sqlc.narg(academic_year_id)::uuid IS NULL OR c.academic_year_id = sqlc.narg(academic_year_id))
  AND (sqlc.narg(status)::text IS NULL OR c.status = sqlc.narg(status));

-- name: GetClass :one
SELECT c.id, c.academic_year_id, ay.name AS academic_year_name, c.name,
       c.grade_level, c.major, c.status, c.created_at, c.updated_at,
       ht.id AS homeroom_teacher_id, COALESCE(ht.full_name, '') AS homeroom_teacher_name
FROM classes c
JOIN academic_years ay ON ay.id = c.academic_year_id
LEFT JOIN LATERAL (
  SELECT t.id, t.full_name
  FROM homeroom_assignments ha
  JOIN teachers t ON t.id = ha.teacher_id
  WHERE ha.class_id = c.id
    AND ha.start_date <= CURRENT_DATE
    AND (ha.end_date IS NULL OR ha.end_date >= CURRENT_DATE)
  ORDER BY ha.start_date DESC
  LIMIT 1
) ht ON true
WHERE c.id = sqlc.arg(id);

-- name: GetClassForUpdate :one
SELECT c.id, c.academic_year_id, c.name, c.grade_level, c.major, c.status,
       ay.start_date AS academic_year_start, ay.end_date AS academic_year_end
FROM classes c
JOIN academic_years ay ON ay.id = c.academic_year_id
WHERE c.id = sqlc.arg(id)
FOR UPDATE OF c;

-- name: GetClassYearDates :one
SELECT ay.start_date, ay.end_date
FROM classes c
JOIN academic_years ay ON ay.id = c.academic_year_id
WHERE c.id = sqlc.arg(id);

-- name: CreateClass :one
INSERT INTO classes (academic_year_id, name, grade_level, major)
VALUES (sqlc.arg(academic_year_id), sqlc.arg(name), sqlc.arg(grade_level), sqlc.narg(major))
RETURNING id;

-- name: UpdateClass :one
UPDATE classes
SET name = sqlc.arg(name), grade_level = sqlc.arg(grade_level), major = sqlc.narg(major), updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id;

-- name: UpdateClassStatus :one
UPDATE classes
SET status = sqlc.arg(status), updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id;

-- name: ListClassStudents :many
SELECT s.id, s.student_number, s.national_student_number, s.full_name, s.gender,
       s.birth_date, s.admission_year, s.status, s.guardian_name, s.guardian_phone,
       s.created_at, s.updated_at,
       e.start_date AS enrollment_start, e.end_date AS enrollment_end
FROM class_enrollments e
JOIN students s ON s.id = e.student_id
WHERE e.class_id = sqlc.arg(class_id)
  AND s.status = 'active'
  AND e.start_date <= CURRENT_DATE
  AND (e.end_date IS NULL OR e.end_date >= CURRENT_DATE)
ORDER BY s.full_name, s.id;

-- name: GetOpenHomeroomForClass :one
SELECT id, teacher_id, class_id, start_date, end_date, created_by, created_at
FROM homeroom_assignments
WHERE class_id = sqlc.arg(class_id) AND end_date IS NULL
ORDER BY start_date DESC
LIMIT 1
FOR UPDATE;

-- name: CloseHomeroomAssignment :exec
UPDATE homeroom_assignments
SET end_date = sqlc.arg(end_date)
WHERE id = sqlc.arg(id);

-- name: CreateHomeroomAssignment :one
INSERT INTO homeroom_assignments (teacher_id, class_id, start_date, created_by)
VALUES (sqlc.arg(teacher_id), sqlc.arg(class_id), sqlc.arg(start_date), sqlc.narg(created_by))
RETURNING id, teacher_id, class_id, start_date, end_date, created_by, created_at;

-- name: GetCurrentHomeroomForClass :one
SELECT ha.id, ha.teacher_id, t.full_name AS teacher_name, ha.class_id,
       ha.start_date, ha.end_date, ha.created_at
FROM homeroom_assignments ha
JOIN teachers t ON t.id = ha.teacher_id
WHERE ha.class_id = sqlc.arg(class_id)
  AND ha.start_date <= CURRENT_DATE
  AND (ha.end_date IS NULL OR ha.end_date >= CURRENT_DATE)
ORDER BY ha.start_date DESC
LIMIT 1;
