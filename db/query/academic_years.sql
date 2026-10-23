-- name: ListAcademicYears :many
SELECT id, name, start_date, end_date, is_active, created_at, updated_at
FROM academic_years
ORDER BY start_date DESC, id;

-- name: GetAcademicYear :one
SELECT id, name, start_date, end_date, is_active, created_at, updated_at
FROM academic_years
WHERE id = sqlc.arg(id);

-- name: GetAcademicYearForUpdate :one
SELECT id, name, start_date, end_date, is_active, created_at, updated_at
FROM academic_years
WHERE id = sqlc.arg(id)
FOR UPDATE;

-- name: CreateAcademicYear :one
INSERT INTO academic_years (name, start_date, end_date, is_active)
VALUES (sqlc.arg(name), sqlc.arg(start_date), sqlc.arg(end_date), sqlc.arg(is_active))
RETURNING id, name, start_date, end_date, is_active, created_at, updated_at;

-- name: UpdateAcademicYear :one
UPDATE academic_years
SET name = sqlc.arg(name),
    start_date = sqlc.arg(start_date),
    end_date = sqlc.arg(end_date),
    is_active = sqlc.arg(is_active),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id, name, start_date, end_date, is_active, created_at, updated_at;

-- name: DeactivateAllAcademicYears :exec
UPDATE academic_years
SET is_active = false, updated_at = now()
WHERE is_active;

-- name: DeactivateOtherAcademicYears :exec
UPDATE academic_years
SET is_active = false, updated_at = now()
WHERE is_active AND id <> sqlc.arg(id);

-- name: LockAcademicYearActivation :exec
SELECT pg_advisory_xact_lock(7632452);

-- name: CountDatedRecordsOutsideAcademicYear :one
SELECT count(*)
FROM (
  SELECT e.id
  FROM class_enrollments e
  JOIN classes c ON c.id = e.class_id
  WHERE c.academic_year_id = sqlc.arg(id)
    AND (
      e.start_date < sqlc.arg(start_date)
      OR e.start_date > sqlc.arg(end_date)
      OR (e.end_date IS NOT NULL AND (e.end_date < sqlc.arg(start_date) OR e.end_date > sqlc.arg(end_date)))
      OR (e.end_date IS NULL AND sqlc.arg(end_date) < CURRENT_DATE)
    )
  UNION ALL
  SELECT ha.id
  FROM homeroom_assignments ha
  JOIN classes c ON c.id = ha.class_id
  WHERE c.academic_year_id = sqlc.arg(id)
    AND (
      ha.start_date < sqlc.arg(start_date)
      OR ha.start_date > sqlc.arg(end_date)
      OR (ha.end_date IS NOT NULL AND (ha.end_date < sqlc.arg(start_date) OR ha.end_date > sqlc.arg(end_date)))
      OR (ha.end_date IS NULL AND sqlc.arg(end_date) < CURRENT_DATE)
    )
) records;
