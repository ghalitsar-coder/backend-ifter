-- name: ListStudents :many
SELECT s.id, s.student_number, s.national_student_number, s.full_name, s.gender,
       s.birth_date, s.admission_year, s.status, s.guardian_name, s.guardian_phone,
       s.created_at, s.updated_at,
       cur.id AS current_class_id, COALESCE(cur.name, '') AS current_class_name
FROM students s
LEFT JOIN LATERAL (
  SELECT c.id, c.name
  FROM class_enrollments e
  JOIN classes c ON c.id = e.class_id
  WHERE e.student_id = s.id
    AND e.start_date <= CURRENT_DATE
    AND (e.end_date IS NULL OR e.end_date >= CURRENT_DATE)
  ORDER BY e.start_date DESC
  LIMIT 1
) cur ON true
WHERE (sqlc.narg(status)::text IS NULL OR s.status = sqlc.narg(status))
  AND (sqlc.narg(class_id)::uuid IS NULL OR cur.id = sqlc.narg(class_id))
  AND (
    sqlc.narg(search)::text IS NULL
    OR s.full_name ILIKE '%' || sqlc.narg(search)::text || '%'
    OR s.student_number ILIKE '%' || sqlc.narg(search)::text || '%'
  )
ORDER BY
  CASE WHEN sqlc.arg(sort_by)::text = 'nis' THEN s.student_number END ASC,
  CASE WHEN sqlc.arg(sort_by)::text = 'name' THEN s.full_name END ASC,
  s.id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountStudents :one
SELECT count(*)
FROM students s
LEFT JOIN LATERAL (
  SELECT c.id
  FROM class_enrollments e
  JOIN classes c ON c.id = e.class_id
  WHERE e.student_id = s.id
    AND e.start_date <= CURRENT_DATE
    AND (e.end_date IS NULL OR e.end_date >= CURRENT_DATE)
  ORDER BY e.start_date DESC
  LIMIT 1
) cur ON true
WHERE (sqlc.narg(status)::text IS NULL OR s.status = sqlc.narg(status))
  AND (sqlc.narg(class_id)::uuid IS NULL OR cur.id = sqlc.narg(class_id))
  AND (
    sqlc.narg(search)::text IS NULL
    OR s.full_name ILIKE '%' || sqlc.narg(search)::text || '%'
    OR s.student_number ILIKE '%' || sqlc.narg(search)::text || '%'
  );

-- name: GetStudent :one
SELECT s.id, s.student_number, s.national_student_number, s.full_name, s.gender,
       s.birth_date, s.admission_year, s.status, s.guardian_name, s.guardian_phone,
       s.created_at, s.updated_at,
       cur.id AS current_class_id, COALESCE(cur.name, '') AS current_class_name
FROM students s
LEFT JOIN LATERAL (
  SELECT c.id, c.name
  FROM class_enrollments e
  JOIN classes c ON c.id = e.class_id
  WHERE e.student_id = s.id
    AND e.start_date <= CURRENT_DATE
    AND (e.end_date IS NULL OR e.end_date >= CURRENT_DATE)
  ORDER BY e.start_date DESC
  LIMIT 1
) cur ON true
WHERE s.id = sqlc.arg(id);

-- name: GetStudentForUpdate :one
SELECT id, student_number, national_student_number, full_name, gender, birth_date,
       admission_year, status, guardian_name, guardian_phone, created_at, updated_at,
       user_id
FROM students
WHERE id = sqlc.arg(id)
FOR UPDATE;

-- name: CreateStudent :one
INSERT INTO students (
  student_number, national_student_number, full_name, gender, birth_date,
  admission_year, status, guardian_name, guardian_phone
)
VALUES (
  sqlc.arg(student_number), sqlc.narg(national_student_number), sqlc.arg(full_name),
  sqlc.narg(gender), sqlc.narg(birth_date), sqlc.arg(admission_year), sqlc.arg(status),
  sqlc.narg(guardian_name), sqlc.narg(guardian_phone)
)
RETURNING id, student_number, national_student_number, full_name, gender, birth_date,
          admission_year, status, guardian_name, guardian_phone, created_at, updated_at,
          user_id;

-- name: UpdateStudent :one
UPDATE students
SET student_number = sqlc.arg(student_number),
    national_student_number = sqlc.narg(national_student_number),
    full_name = sqlc.arg(full_name),
    gender = sqlc.narg(gender),
    birth_date = sqlc.narg(birth_date),
    admission_year = sqlc.arg(admission_year),
    guardian_name = sqlc.narg(guardian_name),
    guardian_phone = sqlc.narg(guardian_phone),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id, student_number, national_student_number, full_name, gender, birth_date,
          admission_year, status, guardian_name, guardian_phone, created_at, updated_at,
          user_id;

-- name: UpdateStudentStatus :one
UPDATE students
SET status = sqlc.arg(status), updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id, student_number, national_student_number, full_name, gender, birth_date,
          admission_year, status, guardian_name, guardian_phone, created_at, updated_at,
          user_id;

-- name: GetOpenEnrollmentForStudentForUpdate :one
SELECT id, student_id, class_id, start_date, end_date, created_by, created_at
FROM class_enrollments
WHERE student_id = sqlc.arg(student_id) AND end_date IS NULL
ORDER BY start_date DESC
LIMIT 1
FOR UPDATE;

-- name: CloseEnrollment :exec
UPDATE class_enrollments
SET end_date = sqlc.arg(end_date)
WHERE id = sqlc.arg(id);

-- name: CreateEnrollment :one
INSERT INTO class_enrollments (student_id, class_id, start_date, created_by)
VALUES (sqlc.arg(student_id), sqlc.arg(class_id), sqlc.arg(start_date), sqlc.narg(created_by))
RETURNING id, student_id, class_id, start_date, end_date, created_by, created_at;

-- name: ListStudentEnrollments :many
SELECT e.id, e.class_id, c.name AS class_name,
       ay.id AS academic_year_id, ay.name AS academic_year_name,
       e.start_date, e.end_date
FROM class_enrollments e
JOIN classes c ON c.id = e.class_id
JOIN academic_years ay ON ay.id = c.academic_year_id
WHERE e.student_id = sqlc.arg(student_id)
ORDER BY e.start_date DESC, e.id;
