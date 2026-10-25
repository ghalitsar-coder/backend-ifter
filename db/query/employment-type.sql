-- Employment-type master data (docs/API_ADDENDUM.md §6.5, migration 000013).

-- name: ListEmploymentTypes :many
SELECT id, code, name, description, is_active, created_at, updated_at
FROM employment_types
ORDER BY code, id;

-- name: GetEmploymentType :one
SELECT id, code, name, description, is_active, created_at, updated_at
FROM employment_types
WHERE id = sqlc.arg(id);

-- name: GetEmploymentTypeForUpdate :one
SELECT id, code, name, description, is_active, created_at, updated_at
FROM employment_types
WHERE id = sqlc.arg(id)
FOR UPDATE;

-- name: GetEmploymentTypeByCode :one
SELECT id, code, name, description, is_active, created_at, updated_at
FROM employment_types
WHERE code = sqlc.arg(code);

-- name: GetEmploymentTypeName :one
SELECT name
FROM employment_types
WHERE id = sqlc.arg(id);

-- name: CreateEmploymentType :one
INSERT INTO employment_types (code, name, description)
VALUES (
  sqlc.arg(code), sqlc.arg(name), sqlc.narg(description)
)
RETURNING id, code, name, description, is_active, created_at, updated_at;

-- name: UpdateEmploymentType :one
UPDATE employment_types
SET code = sqlc.arg(code),
    name = sqlc.arg(name),
    description = sqlc.narg(description),
    is_active = COALESCE(sqlc.narg(next_is_active)::boolean, is_active),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id, code, name, description, is_active, created_at, updated_at;

-- name: SetEmploymentTypeStatus :one
UPDATE employment_types
SET is_active = sqlc.arg(is_active), updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id, code, name, description, is_active, created_at, updated_at;

-- Dashboard breakdown of active teachers by the three contract employment
-- categories (package spec: GET /dashboard gains active_teachers_breakdown,
-- counted server-side over teachers.status = 'active'; unknown = the spec's
-- NULL-backfill bucket, i.e. no (or unresolvable) employment type link).
-- name: DashboardTeacherEmploymentBreakdown :one
SELECT count(*) FILTER (WHERE et.code = 'permanent')::int AS permanent,
       count(*) FILTER (WHERE et.code = 'honorary')::int AS honorary,
       count(*) FILTER (WHERE et.code = 'staff_tu')::int AS staff_tu,
       count(*) FILTER (WHERE et.id IS NULL)::int AS unknown
FROM teachers t
LEFT JOIN employment_types et ON et.id = t.employment_type_id
WHERE t.status = 'active';
