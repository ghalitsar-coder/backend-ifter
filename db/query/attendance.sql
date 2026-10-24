-- name: StaffIDByUserID :one
SELECT id
FROM staff
WHERE user_id = sqlc.arg(user_id)::bigint AND is_active;

-- A guru may record for a class they teach (or are wali kelas of); a specific
-- subject session must belong to them.
-- name: CanRecordAttendance :one
SELECT EXISTS (
    SELECT 1
    FROM classes c
    WHERE c.id = sqlc.arg(class_id)
      AND (
          (sqlc.narg(class_subject_id)::bigint IS NOT NULL AND EXISTS (
              SELECT 1
              FROM class_subjects cs
              WHERE cs.id = sqlc.narg(class_subject_id)
                AND cs.class_id = c.id
                AND cs.teacher_id = sqlc.arg(staff_id)
          ))
          OR
          (sqlc.narg(class_subject_id)::bigint IS NULL AND (
              c.wali_kelas_id = sqlc.arg(staff_id)
              OR EXISTS (
                  SELECT 1
                  FROM class_subjects cs2
                  WHERE cs2.class_id = c.id AND cs2.teacher_id = sqlc.arg(staff_id)
              )
          ))
      )
);

-- name: CreateAttendanceSession :one
WITH inserted AS (
    INSERT INTO attendance_sessions (class_id, class_subject_id, recorded_by, tanggal)
    VALUES (sqlc.arg(class_id), sqlc.narg(class_subject_id), sqlc.arg(recorded_by), sqlc.arg(tanggal))
    ON CONFLICT DO NOTHING
    RETURNING id, class_id, class_subject_id, recorded_by, tanggal, is_locked, created_at
)
SELECT id, class_id, class_subject_id, recorded_by, tanggal, is_locked, created_at FROM inserted
UNION ALL
SELECT s.id, s.class_id, s.class_subject_id, s.recorded_by, s.tanggal, s.is_locked, s.created_at
FROM attendance_sessions s
WHERE s.class_id = sqlc.arg(class_id)
  AND s.tanggal = sqlc.arg(tanggal)
  AND s.class_subject_id IS NOT DISTINCT FROM sqlc.narg(class_subject_id)
LIMIT 1;

-- name: GetAttendanceSession :one
SELECT id, class_id, class_subject_id, recorded_by, tanggal, is_locked, created_at
FROM attendance_sessions
WHERE id = sqlc.arg(id);

-- name: GetAttendanceSessionLockState :one
SELECT is_locked
FROM attendance_sessions
WHERE id = sqlc.arg(id)
FOR UPDATE;

-- name: LockAttendanceSession :one
UPDATE attendance_sessions
SET is_locked = TRUE
WHERE id = sqlc.arg(id)
RETURNING id, class_id, class_subject_id, recorded_by, tanggal, is_locked, created_at;

-- Active roster, left-joined with this session's saved statuses.
-- name: ListSessionAttendances :many
SELECT st.id AS student_id, st.nis, st.nama, sa.status, sa.keterangan
FROM class_enrollments ce
JOIN students st ON st.id = ce.student_id
LEFT JOIN student_attendances sa
    ON sa.session_id = sqlc.arg(session_id) AND sa.student_id = st.id
WHERE ce.class_id = sqlc.arg(class_id)
  AND ce.status = 'aktif'
  AND st.is_active
ORDER BY st.nama, st.id;

-- name: CountEnrolledStudents :one
SELECT count(*)
FROM class_enrollments
WHERE class_id = sqlc.arg(class_id)
  AND status = 'aktif'
  AND student_id = ANY(sqlc.arg(student_ids)::bigint[]);

-- name: UpsertStudentAttendance :exec
INSERT INTO student_attendances (session_id, student_id, status, keterangan)
VALUES (sqlc.arg(session_id), sqlc.arg(student_id), sqlc.arg(status), sqlc.narg(keterangan))
ON CONFLICT (session_id, student_id)
DO UPDATE SET status = EXCLUDED.status, keterangan = EXCLUDED.keterangan;
