-- Duty-rotation package (docs/API_ADDENDUM.md §6.3, migration 000011).

-- name: GetDutyRotation :one
SELECT dr.id, dr.duty_assignment_id, dr.rotation_date, dr.status, dr.reason,
       dr.created_by, dr.created_at, dr.updated_at,
       da.weekday,
       da.teacher_id AS original_teacher_id,
       ot.full_name AS original_teacher_name,
       dr.substitute_teacher_id,
       st.full_name AS substitute_teacher_name
FROM duty_rotations dr
JOIN duty_assignments da ON da.id = dr.duty_assignment_id
JOIN teachers ot ON ot.id = da.teacher_id
JOIN teachers st ON st.id = dr.substitute_teacher_id
WHERE dr.id = sqlc.arg(id);

-- name: ListDutyRotations :many
SELECT dr.id, dr.duty_assignment_id, dr.rotation_date, dr.status, dr.reason,
       dr.created_by, dr.created_at, dr.updated_at,
       da.weekday,
       da.teacher_id AS original_teacher_id,
       ot.full_name AS original_teacher_name,
       dr.substitute_teacher_id,
       st.full_name AS substitute_teacher_name
FROM duty_rotations dr
JOIN duty_assignments da ON da.id = dr.duty_assignment_id
JOIN teachers ot ON ot.id = da.teacher_id
JOIN teachers st ON st.id = dr.substitute_teacher_id
WHERE (sqlc.narg(rotation_date)::date IS NULL OR dr.rotation_date = sqlc.narg(rotation_date))
  AND (sqlc.narg(weekday)::smallint IS NULL OR da.weekday = sqlc.narg(weekday))
  AND (sqlc.narg(relevant_teacher_id)::uuid IS NULL
       OR da.teacher_id = sqlc.narg(relevant_teacher_id)
       OR dr.substitute_teacher_id = sqlc.narg(relevant_teacher_id))
ORDER BY dr.rotation_date DESC, dr.created_at DESC, dr.id;

-- name: GetDutyAssignmentForRotation :one
SELECT da.id, da.teacher_id, da.weekday, da.start_date, da.end_date,
       t.full_name AS teacher_name
FROM duty_assignments da
JOIN teachers t ON t.id = da.teacher_id
WHERE da.id = sqlc.arg(id);

-- name: GetTeacherForRotation :one
SELECT id, full_name, status
FROM teachers
WHERE id = sqlc.arg(id);

-- name: GetStudentForUniform :one
SELECT id, full_name, status
FROM students
WHERE id = sqlc.arg(id);

-- name: CreateDutyRotation :one
INSERT INTO duty_rotations (
  duty_assignment_id, substitute_teacher_id, rotation_date, status, reason, created_by
)
VALUES (
  sqlc.arg(duty_assignment_id), sqlc.arg(substitute_teacher_id),
  sqlc.arg(rotation_date), 'substituted', sqlc.arg(reason), sqlc.arg(created_by)
)
RETURNING id;

-- name: CancelDutyRotation :one
UPDATE duty_rotations
SET status = 'cancelled',
    reason = COALESCE(NULLIF(sqlc.arg(reason)::text, ''), reason),
    updated_at = now()
WHERE id = sqlc.arg(id) AND status = 'substituted'
RETURNING id;

-- name: CreateUniformReport :one
INSERT INTO uniform_reports (for_date, teacher_id, scope, target_student_id, note)
VALUES (
  sqlc.arg(for_date), sqlc.arg(teacher_id), sqlc.arg(scope),
  sqlc.narg(target_student_id), NULLIF(sqlc.arg(note)::text, '')
)
RETURNING id, for_date, teacher_id, scope, target_student_id, note, created_at;

-- name: ListUniformReports :many
SELECT ur.id, ur.for_date, ur.teacher_id, ur.scope, ur.target_student_id, ur.note,
       ur.created_at
FROM uniform_reports ur
WHERE (sqlc.narg(for_date)::date IS NULL OR ur.for_date = sqlc.narg(for_date))
  AND (sqlc.narg(teacher_id)::uuid IS NULL OR ur.teacher_id = sqlc.narg(teacher_id))
ORDER BY ur.for_date DESC, ur.created_at DESC, ur.id;
