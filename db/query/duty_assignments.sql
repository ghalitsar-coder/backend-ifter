-- name: ListDutyAssignments :many
SELECT da.id, da.teacher_id, t.full_name AS teacher_name, da.weekday,
       da.start_date, da.end_date, da.created_at
FROM duty_assignments da
JOIN teachers t ON t.id = da.teacher_id
WHERE (sqlc.narg(teacher_id)::uuid IS NULL OR da.teacher_id = sqlc.narg(teacher_id))
  AND (sqlc.narg(weekday)::smallint IS NULL OR da.weekday = sqlc.narg(weekday))
  AND (
    sqlc.narg(active_date)::date IS NULL
    OR (da.start_date <= sqlc.narg(active_date)::date
        AND (da.end_date IS NULL OR da.end_date >= sqlc.narg(active_date)::date))
  )
ORDER BY da.weekday, t.full_name, da.start_date;

-- name: CreateDutyAssignment :one
INSERT INTO duty_assignments (teacher_id, weekday, start_date, end_date, created_by)
VALUES (sqlc.arg(teacher_id), sqlc.arg(weekday), sqlc.arg(start_date)::date,
        sqlc.narg(end_date)::date, sqlc.arg(created_by))
RETURNING id, teacher_id, weekday, start_date, end_date, created_by, created_at;

-- name: GetDutyAssignmentForUpdate :one
SELECT id, teacher_id, weekday, start_date, end_date, created_by, created_at
FROM duty_assignments
WHERE id = sqlc.arg(id)
FOR UPDATE;

-- name: UpdateDutyAssignmentEnd :one
UPDATE duty_assignments
SET end_date = sqlc.arg(end_date)::date
WHERE id = sqlc.arg(id)
RETURNING id, teacher_id, weekday, start_date, end_date, created_by, created_at;
