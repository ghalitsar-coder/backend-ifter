-- Sanction-rules package (docs/API_ADDENDUM.md §6.4, migration 000012).

-- name: ListSanctionRules :many
SELECT id, name, min_points, max_points, action, level, description,
       is_active, created_at, updated_at
FROM sanction_rules
ORDER BY min_points, id;

-- name: GetSanctionRule :one
SELECT id, name, min_points, max_points, action, level, description,
       is_active, created_at, updated_at
FROM sanction_rules
WHERE id = sqlc.arg(id);

-- name: GetSanctionRuleForUpdate :one
SELECT id, name, min_points, max_points, action, level, description,
       is_active, created_at, updated_at
FROM sanction_rules
WHERE id = sqlc.arg(id)
FOR UPDATE;

-- name: ListActiveSanctionRuleRanges :many
SELECT id, min_points, max_points
FROM sanction_rules
WHERE is_active
ORDER BY min_points, id;

-- name: ListActiveSanctionRuleRangesForUpdate :many
SELECT id, min_points, max_points
FROM sanction_rules
WHERE is_active
ORDER BY min_points, id
FOR UPDATE;

-- name: CreateSanctionRule :one
INSERT INTO sanction_rules (
  name, min_points, max_points, action, level, description, is_active
)
VALUES (
  sqlc.arg(name), sqlc.arg(min_points), sqlc.narg(max_points),
  sqlc.arg(action), sqlc.narg(level), sqlc.narg(description),
  COALESCE(sqlc.narg(is_active)::boolean, true)
)
RETURNING id, name, min_points, max_points, action, level, description,
          is_active, created_at, updated_at;

-- name: UpdateSanctionRule :one
UPDATE sanction_rules
SET name = sqlc.arg(name),
    min_points = sqlc.arg(min_points),
    max_points = sqlc.narg(max_points),
    action = sqlc.arg(action),
    level = sqlc.narg(level),
    description = sqlc.narg(description),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id, name, min_points, max_points, action, level, description,
          is_active, created_at, updated_at;

-- name: SetSanctionRuleStatus :one
UPDATE sanction_rules
SET is_active = sqlc.arg(is_active), updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id, name, min_points, max_points, action, level, description,
          is_active, created_at, updated_at;

-- name: DeleteSanctionRule :one
DELETE FROM sanction_rules
WHERE id = sqlc.arg(id)
RETURNING id;

-- name: CountSanctionRules :one
SELECT count(*) FROM sanction_rules;

-- name: DeleteAllSanctionRules :exec
DELETE FROM sanction_rules;

-- MatchSanctionRuleForPoints resolves the tier for an accumulated points total
-- (GET /discipline/students/:id current_level). Active rules only; the
-- highest-matching min_points wins so a rule set with a gap-free ladder picks
-- the deepest tier. total_points is cast to int: the sum of record points can
-- exceed the smallint rule bounds.
-- name: MatchSanctionRuleForPoints :one
SELECT id, name, min_points, max_points, action, level
FROM sanction_rules
WHERE is_active
  AND min_points::int <= sqlc.arg(total_points)::int
  AND (max_points IS NULL OR max_points::int >= sqlc.arg(total_points)::int)
ORDER BY min_points DESC, id
LIMIT 1;
