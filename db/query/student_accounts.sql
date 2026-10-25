-- student-accounts package (docs/API_ADDENDUM.md §6.7, migration 000015).
-- Only the account-link surface lives here; the /my/* reads reuse the
-- existing attendance/discipline sqlc queries with the student id forced
-- from the authenticated subject (see internal/studentaccounts).

-- name: GetStudentAccount :one
SELECT s.id AS student_id, s.student_number, s.full_name AS student_name,
       s.status AS student_status, s.user_id,
       u.username, u.email, u.full_name AS user_full_name,
       u.status AS user_status, u.last_login_at
FROM students s
LEFT JOIN users u ON u.id = s.user_id
WHERE s.id = sqlc.arg(id);

-- name: GetStudentForAccountLink :one
SELECT id, student_number, full_name, status, user_id
FROM students
WHERE id = sqlc.arg(id)
FOR UPDATE;

-- name: LinkStudentUser :one
UPDATE students
SET user_id = sqlc.arg(user_id), updated_at = now()
WHERE id = sqlc.arg(id) AND user_id IS NULL
RETURNING id, student_number, full_name, status, user_id;

-- name: UnlinkStudentUser :one
UPDATE students
SET user_id = NULL, updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id, student_number, full_name, status, user_id;

-- name: GetActiveStudentByUserID :one
SELECT id, student_number, full_name, status
FROM students
WHERE user_id = sqlc.arg(user_id) AND status = 'active'
LIMIT 1;
