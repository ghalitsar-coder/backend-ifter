-- Dispensations package (docs/API_ADDENDUM.md §6.2, migration 000010).

-- name: GetDispensation :one
SELECT d.id, d.student_id, d.class_id, d.type, d.reason, d.start_date, d.end_date,
       d.status, d.document_file_name, d.document_stored_key,
       d.submitted_by, d.verified_by, d.verified_at, d.note,
       d.created_at, d.updated_at,
       s.full_name AS student_name,
       COALESCE(c.name, '') AS class_name
FROM dispensations d
JOIN students s ON s.id = d.student_id
LEFT JOIN classes c ON c.id = d.class_id
WHERE d.id = sqlc.arg(id);

-- name: ListDispensations :many
SELECT d.id, d.student_id, d.class_id, d.type, d.reason, d.start_date, d.end_date,
       d.status, d.document_file_name, d.document_stored_key,
       d.submitted_by, d.verified_by, d.verified_at, d.note,
       d.created_at, d.updated_at,
       s.full_name AS student_name,
       COALESCE(c.name, '') AS class_name
FROM dispensations d
JOIN students s ON s.id = d.student_id
LEFT JOIN classes c ON c.id = d.class_id
WHERE (sqlc.narg(status)::text IS NULL OR d.status = sqlc.narg(status))
  AND (sqlc.narg(type)::text IS NULL OR d.type = sqlc.narg(type))
  AND (sqlc.narg(student_id)::uuid IS NULL OR d.student_id = sqlc.narg(student_id))
  AND (sqlc.narg(class_id)::uuid IS NULL OR d.class_id = sqlc.narg(class_id))
  AND (sqlc.narg(scope_class_ids)::uuid[] IS NULL OR d.class_id = ANY(sqlc.narg(scope_class_ids)))
  AND (sqlc.narg(from_date)::date IS NULL OR COALESCE(d.end_date, d.start_date) >= sqlc.narg(from_date))
  AND (sqlc.narg(to_date)::date IS NULL OR d.start_date <= sqlc.narg(to_date))
ORDER BY d.start_date DESC, d.created_at DESC, d.id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountDispensations :one
SELECT count(*)
FROM dispensations d
WHERE (sqlc.narg(status)::text IS NULL OR d.status = sqlc.narg(status))
  AND (sqlc.narg(type)::text IS NULL OR d.type = sqlc.narg(type))
  AND (sqlc.narg(student_id)::uuid IS NULL OR d.student_id = sqlc.narg(student_id))
  AND (sqlc.narg(class_id)::uuid IS NULL OR d.class_id = sqlc.narg(class_id))
  AND (sqlc.narg(scope_class_ids)::uuid[] IS NULL OR d.class_id = ANY(sqlc.narg(scope_class_ids)))
  AND (sqlc.narg(from_date)::date IS NULL OR COALESCE(d.end_date, d.start_date) >= sqlc.narg(from_date))
  AND (sqlc.narg(to_date)::date IS NULL OR d.start_date <= sqlc.narg(to_date));

-- name: CreateDispensation :one
INSERT INTO dispensations (
  student_id, class_id, type, reason, start_date, end_date,
  document_file_name, document_stored_key, submitted_by
)
VALUES (
  sqlc.arg(student_id), sqlc.narg(class_id), sqlc.arg(type), sqlc.arg(reason),
  sqlc.arg(start_date), sqlc.narg(end_date),
  sqlc.narg(document_file_name), sqlc.narg(document_stored_key), sqlc.arg(submitted_by)
)
RETURNING id, student_id, class_id, type, reason, start_date, end_date, status,
          document_file_name, document_stored_key, submitted_by, verified_by,
          verified_at, note, created_at, updated_at;

-- name: VerifyDispensation :one
UPDATE dispensations
SET status = sqlc.arg(status),
    verified_by = sqlc.arg(verified_by),
    verified_at = now(),
    note = NULLIF(sqlc.arg(note)::text, ''),
    updated_at = now()
WHERE id = sqlc.arg(id) AND status = 'pending'
RETURNING id, student_id, class_id, type, reason, start_date, end_date, status,
          document_file_name, document_stored_key, submitted_by, verified_by,
          verified_at, note, created_at, updated_at;

-- name: GetStudentCurrentClass :one
SELECT c.id, c.name
FROM class_enrollments e
JOIN classes c ON c.id = e.class_id
WHERE e.student_id = sqlc.arg(student_id)
  AND e.start_date <= CURRENT_DATE
  AND (e.end_date IS NULL OR e.end_date >= CURRENT_DATE)
ORDER BY e.start_date DESC
LIMIT 1;

-- name: ListCurrentHomeroomClassIDs :many
SELECT ha.class_id
FROM homeroom_assignments ha
WHERE ha.teacher_id = sqlc.arg(teacher_id)
  AND ha.start_date <= CURRENT_DATE
  AND (ha.end_date IS NULL OR ha.end_date >= CURRENT_DATE);
