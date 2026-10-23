-- name: GetUserByUsername :one
SELECT id, username, email, password_hash, full_name, status, last_login_at, created_at, updated_at
FROM users
WHERE lower(username) = lower(sqlc.arg(username))
LIMIT 1;

-- name: GetUserByEmail :one
SELECT id, username, email, password_hash, full_name, status, last_login_at, created_at, updated_at
FROM users
WHERE email IS NOT NULL
  AND lower(email) = lower(sqlc.arg(email))
LIMIT 1;

-- name: GetUserByID :one
SELECT id, username, email, password_hash, full_name, status, last_login_at, created_at, updated_at
FROM users
WHERE id = sqlc.arg(id);

-- name: ListUserRoles :many
SELECT r.code, r.name, r.description
FROM user_roles ur
JOIN roles r ON r.code = ur.role_code
WHERE ur.user_id = sqlc.arg(user_id)
ORDER BY r.code;

-- name: UpdateLastLogin :exec
UPDATE users
SET last_login_at = now(), updated_at = now()
WHERE id = sqlc.arg(id);

-- name: CreateUser :one
INSERT INTO users (username, email, password_hash, full_name, status)
VALUES (
  sqlc.arg(username), sqlc.narg(email), sqlc.arg(password_hash),
  sqlc.arg(full_name), sqlc.arg(status)
)
RETURNING id, username, email, password_hash, full_name, status, last_login_at, created_at, updated_at;

-- name: ListUsers :many
SELECT id, username, email, full_name, status, last_login_at, created_at
FROM users
WHERE (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
ORDER BY created_at DESC, id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountUsers :one
SELECT count(*)
FROM users
WHERE (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status));

-- name: CountUsersWithRole :one
SELECT count(*)
FROM user_roles
WHERE role_code = sqlc.arg(role_code);

-- name: AssignUserRole :exec
INSERT INTO user_roles (user_id, role_code, assigned_by)
VALUES (sqlc.arg(user_id), sqlc.arg(role_code), sqlc.narg(assigned_by))
ON CONFLICT (user_id, role_code) DO NOTHING;

-- name: GetUserByIDForUpdate :one
SELECT id, username, email, password_hash, full_name, status, last_login_at, created_at, updated_at
FROM users
WHERE id = sqlc.arg(id)
FOR UPDATE;

-- name: UpdateUserStatus :one
UPDATE users
SET status = sqlc.arg(status), updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id, username, email, password_hash, full_name, status, last_login_at, created_at, updated_at;

-- name: UpdateUserPassword :one
UPDATE users
SET password_hash = sqlc.arg(password_hash), updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id, username, email, password_hash, full_name, status, last_login_at, created_at, updated_at;

-- name: DeleteUserRoles :exec
DELETE FROM user_roles
WHERE user_id = sqlc.arg(user_id);

-- name: CountActiveAdmins :one
SELECT count(*)
FROM user_roles ur
JOIN users u ON u.id = ur.user_id
WHERE ur.role_code = 'admin' AND u.status = 'active';

-- name: ListRoles :many
SELECT code, name, description
FROM roles
ORDER BY code;

-- name: LockAdminGuard :exec
SELECT pg_advisory_xact_lock(7632451);
