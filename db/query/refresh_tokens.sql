-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
VALUES (sqlc.arg(user_id), sqlc.arg(token_hash), sqlc.arg(expires_at))
RETURNING id, user_id, token_hash, expires_at, revoked_at, replaced_by, created_at;

-- name: GetRefreshTokenByHash :one
SELECT id, user_id, token_hash, expires_at, revoked_at, replaced_by, created_at
FROM refresh_tokens
WHERE token_hash = sqlc.arg(token_hash);

-- name: GetRefreshTokenByHashForUpdate :one
SELECT id, user_id, token_hash, expires_at, revoked_at, replaced_by, created_at
FROM refresh_tokens
WHERE token_hash = sqlc.arg(token_hash)
FOR UPDATE;

-- name: RotateRefreshToken :exec
UPDATE refresh_tokens
SET revoked_at = now(), replaced_by = sqlc.arg(replaced_by)
WHERE id = sqlc.arg(id) AND revoked_at IS NULL;

-- name: RevokeRefreshToken :exec
UPDATE refresh_tokens
SET revoked_at = now()
WHERE id = sqlc.arg(id) AND revoked_at IS NULL;

-- name: RevokeAllUserRefreshTokens :exec
UPDATE refresh_tokens
SET revoked_at = now()
WHERE user_id = sqlc.arg(user_id) AND revoked_at IS NULL;
