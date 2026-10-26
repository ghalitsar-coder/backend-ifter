-- RFID tap ingest (F11, migration 000021): card mapping and the tap log.

-- name: GetStudentCard :one
SELECT student_id, card_uid, created_at, updated_at
FROM student_cards
WHERE student_id = sqlc.arg(student_id);

-- name: UpsertStudentCard :one
INSERT INTO student_cards (student_id, card_uid)
VALUES (sqlc.arg(student_id), sqlc.arg(card_uid))
ON CONFLICT (student_id) DO UPDATE
SET card_uid = EXCLUDED.card_uid, updated_at = now()
RETURNING student_id, card_uid, created_at, updated_at;

-- name: DeleteStudentCard :execrows
DELETE FROM student_cards WHERE student_id = sqlc.arg(student_id);

-- name: GetStudentsByCardUIDs :many
SELECT c.card_uid, s.id AS student_id
FROM student_cards c
JOIN students s ON s.id = c.student_id
WHERE c.card_uid = ANY(sqlc.arg(card_uids)::text[]);

-- name: CreateDeviceTap :execrows
INSERT INTO device_taps (device_id, student_id, card_uid, occurred_at)
VALUES (
  sqlc.arg(device_id), sqlc.narg(student_id), sqlc.arg(card_uid), sqlc.arg(occurred_at)
)
ON CONFLICT (device_id, card_uid, occurred_at) DO NOTHING;

-- name: ListDeviceTaps :many
SELECT t.id, t.device_id, d.name AS device_name, d.identifier AS device_identifier,
       t.student_id, st.student_number, st.full_name,
       t.card_uid, t.occurred_at, t.created_at
FROM device_taps t
JOIN devices d ON d.id = t.device_id
LEFT JOIN students st ON st.id = t.student_id
WHERE (sqlc.narg(device_id)::uuid IS NULL OR t.device_id = sqlc.narg(device_id)::uuid)
  AND (sqlc.narg(student_id)::uuid IS NULL OR t.student_id = sqlc.narg(student_id)::uuid)
  AND (sqlc.narg(card_uid)::text IS NULL OR t.card_uid = sqlc.narg(card_uid)::text)
  AND (sqlc.narg(from_date)::date IS NULL OR t.occurred_at >= sqlc.narg(from_date)::date)
  AND (sqlc.narg(to_date)::date IS NULL OR t.occurred_at < (sqlc.narg(to_date)::date + interval '1 day'))
ORDER BY t.occurred_at DESC, t.id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountDeviceTaps :one
SELECT count(*)
FROM device_taps t
WHERE (sqlc.narg(device_id)::uuid IS NULL OR t.device_id = sqlc.narg(device_id)::uuid)
  AND (sqlc.narg(student_id)::uuid IS NULL OR t.student_id = sqlc.narg(student_id)::uuid)
  AND (sqlc.narg(card_uid)::text IS NULL OR t.card_uid = sqlc.narg(card_uid)::text)
  AND (sqlc.narg(from_date)::date IS NULL OR t.occurred_at >= sqlc.narg(from_date)::date)
  AND (sqlc.narg(to_date)::date IS NULL OR t.occurred_at < (sqlc.narg(to_date)::date + interval '1 day'));
