-- db/query/discipline.sql
-- Modul Kedisiplinan Siswa (Kelompok D-03)

-- name: ListViolationTypes :many
SELECT id, school_id, kode, nama, kategori, poin_default, is_active, created_at
FROM violation_types
WHERE school_id = sqlc.arg(school_id) AND is_active
ORDER BY kategori, kode;

-- name: GetViolationTypeByID :one
SELECT id, school_id, kode, nama, kategori, poin_default, is_active, created_at
FROM violation_types
WHERE id = sqlc.arg(id) AND school_id = sqlc.arg(school_id);

-- name: CreateViolationType :one
INSERT INTO violation_types (school_id, kode, nama, kategori, poin_default)
VALUES (sqlc.arg(school_id), sqlc.arg(kode), sqlc.arg(nama), sqlc.arg(kategori), sqlc.arg(poin_default))
RETURNING id, school_id, kode, nama, kategori, poin_default, is_active, created_at;

-- name: CreateDisciplineInspection :one
WITH inserted AS (
    INSERT INTO discipline_inspections (school_id, class_id, inspector_id, tanggal, jumlah_diperiksa, catatan)
    VALUES (sqlc.arg(school_id), sqlc.arg(class_id), sqlc.arg(inspector_id), sqlc.arg(tanggal), sqlc.arg(jumlah_diperiksa), sqlc.narg(catatan))
    ON CONFLICT (class_id, tanggal) DO UPDATE
    SET jumlah_diperiksa = EXCLUDED.jumlah_diperiksa,
        catatan = EXCLUDED.catatan
    RETURNING id, school_id, class_id, inspector_id, tanggal, jumlah_diperiksa, catatan, created_at
)
SELECT id, school_id, class_id, inspector_id, tanggal, jumlah_diperiksa, catatan, created_at FROM inserted;

-- name: GetDisciplineInspection :one
SELECT di.id, di.school_id, di.class_id, di.inspector_id, di.tanggal, di.jumlah_diperiksa, di.catatan, di.created_at,
       c.nama_kelas, st.nama AS inspector_name
FROM discipline_inspections di
JOIN classes c ON c.id = di.class_id
JOIN staff st ON st.id = di.inspector_id
WHERE di.id = sqlc.arg(id) AND di.school_id = sqlc.arg(school_id);

-- name: ListDisciplineInspections :many
SELECT di.id, di.school_id, di.class_id, di.inspector_id, di.tanggal, di.jumlah_diperiksa, di.catatan, di.created_at,
       c.nama_kelas, st.nama AS inspector_name
FROM discipline_inspections di
JOIN classes c ON c.id = di.class_id
JOIN staff st ON st.id = di.inspector_id
WHERE di.school_id = sqlc.arg(school_id)
  AND (sqlc.narg(class_id)::bigint IS NULL OR di.class_id = sqlc.narg(class_id))
  AND (sqlc.narg(tanggal)::date IS NULL OR di.tanggal = sqlc.narg(tanggal))
ORDER BY di.tanggal DESC, di.created_at DESC;

-- name: RecordStudentViolation :one
INSERT INTO student_violations (school_id, inspection_id, student_id, violation_type_id, poin, keterangan, tindak_lanjut, tanggal)
VALUES (sqlc.arg(school_id), sqlc.narg(inspection_id), sqlc.arg(student_id), sqlc.arg(violation_type_id), sqlc.arg(poin), sqlc.narg(keterangan), sqlc.narg(tindak_lanjut), sqlc.arg(tanggal))
RETURNING id, school_id, inspection_id, student_id, violation_type_id, poin, keterangan, tindak_lanjut, tanggal, created_at;

-- name: ListStudentViolations :many
SELECT sv.id, sv.school_id, sv.inspection_id, sv.student_id, sv.violation_type_id, sv.poin, sv.keterangan, sv.tindak_lanjut, sv.tanggal, sv.created_at,
       s.nis, s.nama AS student_name, c.nama_kelas,
       vt.kode AS violation_code, vt.nama AS violation_name, vt.kategori AS violation_category
FROM student_violations sv
JOIN students s ON s.id = sv.student_id
LEFT JOIN class_enrollments ce ON ce.student_id = s.id AND ce.status = 'aktif'
LEFT JOIN classes c ON c.id = ce.class_id
JOIN violation_types vt ON vt.id = sv.violation_type_id
WHERE sv.school_id = sqlc.arg(school_id)
  AND (sqlc.narg(student_id)::bigint IS NULL OR sv.student_id = sqlc.narg(student_id))
  AND (sqlc.narg(class_id)::bigint IS NULL OR c.id = sqlc.narg(class_id))
  AND (sqlc.narg(kategori)::varchar IS NULL OR vt.kategori = sqlc.narg(kategori))
ORDER BY sv.tanggal DESC, sv.created_at DESC;

-- name: GetStudentPointsSummary :one
SELECT COALESCE(SUM(sv.poin), 0)::bigint AS total_points,
       COUNT(sv.id)::bigint AS total_incidents
FROM student_violations sv
WHERE sv.student_id = sqlc.arg(student_id)
  AND sv.school_id = sqlc.arg(school_id);

-- name: ListClassStudentsForInspection :many
SELECT s.id, s.nis, s.nama, s.jenis_kelamin, c.nama_kelas
FROM class_enrollments ce
JOIN students s ON s.id = ce.student_id
JOIN classes c ON c.id = ce.class_id
JOIN academic_years ay ON ay.id = c.academic_year_id
WHERE ce.class_id = sqlc.arg(class_id)
  AND ay.school_id = sqlc.arg(school_id)
  AND ce.status = 'aktif'
  AND s.is_active
ORDER BY s.nama;

-- name: UpdateStudentViolationFollowup :one
UPDATE student_violations
SET tindak_lanjut = sqlc.arg(tindak_lanjut)
WHERE id = sqlc.arg(id) AND school_id = sqlc.arg(school_id)
RETURNING id, school_id, inspection_id, student_id, violation_type_id, poin, keterangan, tindak_lanjut, tanggal, created_at;
