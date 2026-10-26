-- name: CreateAuditLog :one
INSERT INTO audit_logs (
  actor_user_id, action, entity_type, entity_id, request_id, before_data, after_data, reason
)
VALUES (
  sqlc.arg(actor_user_id), sqlc.arg(action), sqlc.arg(entity_type), sqlc.arg(entity_id),
  sqlc.arg(request_id), NULLIF(sqlc.arg(before_data)::text, '')::jsonb, NULLIF(sqlc.arg(after_data)::text, '')::jsonb, sqlc.arg(reason)
)
RETURNING id, created_at;

-- name: DeleteAuditLogsBefore :execrows
-- Requires SET LOCAL ifter.audit_retention = 'on' in the same transaction.
DELETE FROM audit_logs
WHERE created_at < sqlc.arg(cutoff);
