-- correction-evidence package (docs/API_ADDENDUM.md §6.6, migration 000014).
-- Attachments supporting an attendance correction. The canonical
-- evidence_hash written into attendance_corrections is produced by the
-- correction flow itself (see the two edited queries in attendance.sql).

-- name: TransactionNow :one
-- The stable transaction timestamp (now() is constant per transaction). The
-- correction flow hashes over this value and inserts it explicitly so the
-- prevent_closed_attendance_update trigger (corrected_at = now()) still fires.
SELECT CAST(now() AS timestamptz);

-- name: GetAttendanceRecordForEvidence :one
SELECT ar.id, e.class_id, ar.attendance_date
FROM attendance_records ar
JOIN class_enrollments e ON e.id = ar.enrollment_id
WHERE ar.id = sqlc.arg(attendance_record_id);

-- name: LatestAttendanceCorrectionForRecord :one
SELECT id
FROM attendance_corrections
WHERE attendance_record_id = sqlc.arg(attendance_record_id)
ORDER BY corrected_at DESC, id
LIMIT 1;

-- name: GetAttendanceCorrectionForRecord :one
SELECT id
FROM attendance_corrections
WHERE id = sqlc.arg(id) AND attendance_record_id = sqlc.arg(attendance_record_id);

-- name: CreateCorrectionEvidence :one
INSERT INTO attendance_correction_evidence (
  attendance_record_id, correction_id, file_name, content_type, size_bytes,
  stored_key, uploaded_by, note
)
VALUES (
  sqlc.arg(attendance_record_id), sqlc.narg(correction_id), sqlc.arg(file_name),
  sqlc.arg(content_type), sqlc.arg(size_bytes), sqlc.arg(stored_key),
  sqlc.arg(uploaded_by), sqlc.narg(note)
)
RETURNING id, attendance_record_id, correction_id, file_name, content_type,
          size_bytes, stored_key, uploaded_by, note, created_at;

-- name: ListCorrectionEvidence :many
SELECT id, attendance_record_id, correction_id, file_name, content_type,
       size_bytes, uploaded_by, note, created_at
FROM attendance_correction_evidence
WHERE attendance_record_id = sqlc.arg(attendance_record_id)
ORDER BY created_at DESC, id;

-- name: GetCorrectionEvidence :one
SELECT id, attendance_record_id, correction_id, file_name, content_type,
       size_bytes, stored_key, uploaded_by, note, created_at
FROM attendance_correction_evidence
WHERE id = sqlc.arg(id);

-- name: DeleteCorrectionEvidence :exec
DELETE FROM attendance_correction_evidence WHERE id = sqlc.arg(id);
