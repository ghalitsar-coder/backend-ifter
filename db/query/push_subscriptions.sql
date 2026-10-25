-- Web-push package (F9, migration 000018). Keys are only read by the send
-- path; the list API never returns p256dh/auth.

-- name: ListPushSubscriptionsByUser :many
SELECT id, user_id, endpoint, p256dh, auth, user_agent, created_at, updated_at
FROM push_subscriptions
WHERE user_id = sqlc.arg(user_id)
ORDER BY created_at DESC, id;

-- name: UpsertPushSubscription :one
INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth, user_agent)
VALUES (
  sqlc.arg(user_id), sqlc.arg(endpoint), sqlc.arg(p256dh), sqlc.arg(auth),
  sqlc.narg(user_agent)
)
ON CONFLICT (endpoint) DO UPDATE
SET user_id = EXCLUDED.user_id,
    p256dh = EXCLUDED.p256dh,
    auth = EXCLUDED.auth,
    user_agent = EXCLUDED.user_agent,
    updated_at = now()
RETURNING id, user_id, endpoint, p256dh, auth, user_agent, created_at, updated_at;

-- name: DeletePushSubscriptionForUser :execrows
DELETE FROM push_subscriptions
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- name: DeletePushSubscription :exec
DELETE FROM push_subscriptions WHERE id = sqlc.arg(id);

-- name: ListPushSubscriptionsForRoles :many
SELECT DISTINCT ps.id, ps.user_id, ps.endpoint, ps.p256dh, ps.auth
FROM push_subscriptions ps
JOIN user_roles ur ON ur.user_id = ps.user_id
WHERE ur.role_code = ANY(sqlc.arg(roles)::text[])
ORDER BY ps.id;
