-- name: ListStudentNotes :many
SELECT n.id, n.student_id, n.note, n.created_by,
       COALESCE(u.full_name, '') AS created_by_name, n.created_at
FROM student_notes n
LEFT JOIN users u ON u.id = n.created_by
WHERE n.student_id = sqlc.arg(student_id)
ORDER BY n.created_at DESC, n.id;

-- name: GetStudentNote :one
SELECT n.id, n.student_id, n.note, n.created_by,
       COALESCE(u.full_name, '') AS created_by_name, n.created_at
FROM student_notes n
LEFT JOIN users u ON u.id = n.created_by
WHERE n.id = sqlc.arg(id) AND n.student_id = sqlc.arg(student_id);

-- name: CreateStudentNote :one
WITH inserted AS (
  INSERT INTO student_notes (student_id, note, created_by)
  VALUES (sqlc.arg(student_id), sqlc.arg(note), sqlc.narg(created_by))
  RETURNING id, student_id, note, created_by, created_at
)
SELECT i.id, i.student_id, i.note, i.created_by,
       COALESCE(u.full_name, '') AS created_by_name, i.created_at
FROM inserted i
LEFT JOIN users u ON u.id = i.created_by;

-- name: DeleteStudentNote :execrows
DELETE FROM student_notes
WHERE id = sqlc.arg(id) AND student_id = sqlc.arg(student_id);
