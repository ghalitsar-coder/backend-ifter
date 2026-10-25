-- Parent portal (migration 000019): the parent_students link and the reads
-- behind /parent/children*. Attendance/discipline reads reuse the shared
-- queries with the child's student id forced after the link check.

-- name: ListParentLinks :many
SELECT ps.user_id, s.id AS student_id, s.student_number, s.full_name, s.status,
       ps.relationship, ps.created_at AS linked_at,
       cur.id AS current_class_id, COALESCE(cur.name, '') AS current_class_name
FROM parent_students ps
JOIN students s ON s.id = ps.student_id
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
WHERE ps.user_id = sqlc.arg(user_id)
ORDER BY s.full_name, s.id;

-- name: GetParentChild :one
SELECT ps.user_id, s.id AS student_id, s.student_number, s.full_name, s.status,
       ps.relationship, ps.created_at AS linked_at,
       cur.id AS current_class_id, COALESCE(cur.name, '') AS current_class_name
FROM parent_students ps
JOIN students s ON s.id = ps.student_id
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
WHERE ps.user_id = sqlc.arg(user_id) AND ps.student_id = sqlc.arg(student_id);

-- name: DeleteParentLinksForUser :exec
DELETE FROM parent_students WHERE user_id = sqlc.arg(user_id);

-- name: InsertParentLink :exec
INSERT INTO parent_students (user_id, student_id, relationship)
VALUES (sqlc.arg(user_id), sqlc.arg(student_id), sqlc.narg(relationship))
ON CONFLICT (user_id, student_id)
DO UPDATE SET relationship = EXCLUDED.relationship;
