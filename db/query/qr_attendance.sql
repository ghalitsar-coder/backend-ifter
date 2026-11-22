-- name: CreateQRSession :one
INSERT INTO qr_sessions (
    school_id, created_by, token_qr, tanggal, tipe_sesi,
    jam_buka, jam_tutup, batas_terlambat, rotasi_detik
)
VALUES (
    sqlc.arg(school_id),
    sqlc.arg(created_by),
    sqlc.arg(token_qr),
    sqlc.arg(tanggal),
    sqlc.arg(tipe_sesi),
    sqlc.arg(jam_buka)::text::time,
    sqlc.arg(jam_tutup)::text::time,
    sqlc.narg(batas_terlambat)::text::time,
    sqlc.arg(rotasi_detik)
)
RETURNING
    id, school_id, created_by, token_qr, tanggal, tipe_sesi,
    to_char(jam_buka, 'HH24:MI:SS') AS jam_buka,
    to_char(jam_tutup, 'HH24:MI:SS') AS jam_tutup,
    coalesce(to_char(batas_terlambat, 'HH24:MI:SS'), '')::varchar AS batas_terlambat,
    rotasi_detik, status, created_at;

-- name: GetQRSession :one
SELECT
    id, school_id, created_by, token_qr, tanggal, tipe_sesi,
    to_char(jam_buka, 'HH24:MI:SS') AS jam_buka,
    to_char(jam_tutup, 'HH24:MI:SS') AS jam_tutup,
    coalesce(to_char(batas_terlambat, 'HH24:MI:SS'), '')::varchar AS batas_terlambat,
    rotasi_detik, status, created_at
FROM qr_sessions
WHERE id = sqlc.arg(id) AND school_id = sqlc.arg(school_id);

-- name: FindOpenQRSession :one
SELECT id
FROM qr_sessions
WHERE school_id = sqlc.arg(school_id)
  AND status = 'aktif'
  AND (tanggal > sqlc.arg(tanggal) OR
       (tanggal = sqlc.arg(tanggal) AND jam_tutup >= sqlc.arg(clock)::text::time))
ORDER BY (tanggal = sqlc.arg(tanggal) AND jam_buka <= sqlc.arg(clock)::text::time) DESC,
         tanggal, jam_buka, created_at DESC, id DESC
LIMIT 1;

-- name: FindActiveStaffForSchool :one
SELECT st.id
FROM staff st
JOIN users u ON u.id = st.user_id
WHERE u.id = sqlc.arg(user_id)
  AND u.school_id = sqlc.arg(school_id)
  AND st.is_active;

-- name: RecordStaffCheckIn :one
INSERT INTO staff_attendances (
    staff_id, masuk_session_id, tanggal, jam_masuk, status, metode
)
VALUES (
    sqlc.arg(staff_id), sqlc.arg(session_id), sqlc.arg(tanggal),
    sqlc.arg(scanned_at), sqlc.arg(status), 'qr'
)
ON CONFLICT (staff_id, tanggal) DO NOTHING
RETURNING id, staff_id, tanggal, jam_masuk, jam_pulang, status;

-- name: RecordStaffCheckOut :one
UPDATE staff_attendances
SET pulang_session_id = sqlc.arg(session_id),
    jam_pulang = sqlc.arg(scanned_at)
WHERE staff_id = sqlc.arg(staff_id)
  AND tanggal = sqlc.arg(tanggal)
  AND jam_masuk IS NOT NULL
  AND jam_pulang IS NULL
RETURNING id, staff_id, tanggal, jam_masuk, jam_pulang, status;

-- name: CloseQRSession :one
UPDATE qr_sessions
SET status = 'selesai'
WHERE id = sqlc.arg(id)
  AND school_id = sqlc.arg(school_id)
  AND status = 'aktif'
RETURNING id, status;
