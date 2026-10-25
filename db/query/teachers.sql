-- name: ListTeachers :many
SELECT id, user_id, employee_number, full_name, email, phone, status, created_at, updated_at, employment_type_id
FROM teachers
WHERE (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (
    sqlc.narg(search)::text IS NULL
    OR full_name ILIKE '%' || sqlc.narg(search)::text || '%'
    OR employee_number ILIKE '%' || sqlc.narg(search)::text || '%'
  )
ORDER BY full_name, id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountTeachers :one
SELECT count(*)
FROM teachers
WHERE (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (
    sqlc.narg(search)::text IS NULL
    OR full_name ILIKE '%' || sqlc.narg(search)::text || '%'
    OR employee_number ILIKE '%' || sqlc.narg(search)::text || '%'
  );

-- name: GetTeacher :one
SELECT id, user_id, employee_number, full_name, email, phone, status, created_at, updated_at, employment_type_id
FROM teachers
WHERE id = sqlc.arg(id);

-- name: GetTeacherForUpdate :one
SELECT id, user_id, employee_number, full_name, email, phone, status, created_at, updated_at, employment_type_id
FROM teachers
WHERE id = sqlc.arg(id)
FOR UPDATE;

-- name: CreateTeacher :one
INSERT INTO teachers (user_id, employee_number, full_name, email, phone, status, employment_type_id)
VALUES (
  sqlc.narg(user_id), sqlc.narg(employee_number), sqlc.arg(full_name),
  sqlc.narg(email), sqlc.narg(phone), sqlc.arg(status), sqlc.narg(employment_type_id)
)
RETURNING id, user_id, employee_number, full_name, email, phone, status, created_at, updated_at, employment_type_id;

-- name: UpdateTeacher :one
UPDATE teachers
SET user_id = sqlc.narg(user_id),
    employee_number = sqlc.narg(employee_number),
    full_name = sqlc.arg(full_name),
    email = sqlc.narg(email),
    phone = sqlc.narg(phone),
    employment_type_id = sqlc.narg(employment_type_id),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id, user_id, employee_number, full_name, email, phone, status, created_at, updated_at, employment_type_id;

-- name: UpdateTeacherStatus :one
UPDATE teachers
SET status = sqlc.arg(status), updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id, user_id, employee_number, full_name, email, phone, status, created_at, updated_at, employment_type_id;
