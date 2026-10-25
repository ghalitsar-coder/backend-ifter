-- Device-registry package (F10, migration 000017). last_seen_at stays NULL
-- until the RFID ingest surface (F11) starts touching devices.

-- name: ListDevices :many
SELECT id, name, identifier, device_type, location, notes, status,
       last_seen_at, created_at, updated_at, ingest_token_hash
FROM devices
WHERE (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(device_type)::text IS NULL OR device_type = sqlc.narg(device_type))
ORDER BY name, id;

-- name: GetDevice :one
SELECT id, name, identifier, device_type, location, notes, status,
       last_seen_at, created_at, updated_at, ingest_token_hash
FROM devices
WHERE id = sqlc.arg(id);

-- name: GetDeviceForUpdate :one
SELECT id, name, identifier, device_type, location, notes, status,
       last_seen_at, created_at, updated_at, ingest_token_hash
FROM devices
WHERE id = sqlc.arg(id)
FOR UPDATE;

-- name: CreateDevice :one
INSERT INTO devices (name, identifier, device_type, location, notes, ingest_token_hash)
VALUES (
  sqlc.arg(name), sqlc.arg(identifier), sqlc.arg(device_type),
  sqlc.narg(location), sqlc.narg(notes), sqlc.arg(ingest_token_hash)
)
RETURNING id, name, identifier, device_type, location, notes, status,
          last_seen_at, created_at, updated_at, ingest_token_hash;

-- name: GetDeviceByIngestTokenHash :one
SELECT id, name, identifier, device_type, location, notes, status,
       last_seen_at, created_at, updated_at, ingest_token_hash
FROM devices
WHERE ingest_token_hash = sqlc.arg(ingest_token_hash);

-- name: RotateDeviceToken :one
UPDATE devices
SET ingest_token_hash = sqlc.arg(ingest_token_hash), updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id;

-- name: TouchDeviceLastSeen :exec
UPDATE devices
SET last_seen_at = now(), updated_at = now()
WHERE id = sqlc.arg(id);

-- name: UpdateDevice :one
UPDATE devices
SET name = sqlc.arg(name),
    identifier = sqlc.arg(identifier),
    device_type = sqlc.arg(device_type),
    location = sqlc.narg(location),
    notes = sqlc.narg(notes),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id, name, identifier, device_type, location, notes, status,
          last_seen_at, created_at, updated_at, ingest_token_hash;

-- name: UpdateDeviceStatus :one
UPDATE devices
SET status = sqlc.arg(status), updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id, name, identifier, device_type, location, notes, status,
          last_seen_at, created_at, updated_at, ingest_token_hash;
