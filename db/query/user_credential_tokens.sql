-- name: CreateCredentialToken :one
INSERT INTO user_credential_tokens (user_id, purpose, token_hash, expires_at, created_by)
VALUES (
  sqlc.arg(user_id), sqlc.arg(purpose), sqlc.arg(token_hash),
  sqlc.arg(expires_at), sqlc.narg(created_by)
)
RETURNING id, user_id, purpose, token_hash, expires_at, used_at, created_by, created_at;

-- name: GetCredentialTokenByHashForUpdate :one
SELECT id, user_id, purpose, token_hash, expires_at, used_at, created_by, created_at
FROM user_credential_tokens
WHERE token_hash = sqlc.arg(token_hash)
FOR UPDATE;

-- name: MarkCredentialTokenUsed :exec
UPDATE user_credential_tokens
SET used_at = now()
WHERE id = sqlc.arg(id) AND used_at IS NULL;

-- name: DeleteUnusedCredentialTokens :exec
DELETE FROM user_credential_tokens
WHERE user_id = sqlc.arg(user_id) AND used_at IS NULL;

-- name: DeleteOtherUnusedCredentialTokens :exec
DELETE FROM user_credential_tokens
WHERE user_id = sqlc.arg(user_id)
  AND id <> sqlc.arg(keep_id)
  AND used_at IS NULL;
