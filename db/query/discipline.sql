-- db/query/discipline.sql
-- Modul Kedisiplinan Siswa (Kelompok D-03)

-- name: ListViolationTypes :many
SELECT id, school_id, kode, nama, kategori, poin_default, is_active, created_at
FROM violation_types
WHERE school_id = sqlc.arg(school_id)
  AND (sqlc.arg(include_inactive)::boolean OR is_active)
ORDER BY kategori, kode;

-- name: GetViolationTypeByID :one
SELECT id, school_id, kode, nama, kategori, poin_default, is_active, created_at
FROM violation_types
WHERE id = sqlc.arg(id) AND school_id = sqlc.arg(school_id);

-- name: CreateViolationType :one
INSERT INTO violation_types (school_id, kode, nama, kategori, poin_default, is_active)
VALUES (sqlc.arg(school_id), sqlc.arg(kode), sqlc.arg(nama), sqlc.arg(kategori), sqlc.arg(poin_default), sqlc.arg(is_active))
RETURNING id, school_id, kode, nama, kategori, poin_default, is_active, created_at;

-- name: UpdateViolationType :one
UPDATE violation_types
SET kode = sqlc.arg(kode),
    nama = sqlc.arg(nama),
    kategori = sqlc.arg(kategori),
    poin_default = sqlc.arg(poin_default),
    is_active = sqlc.arg(is_active)
WHERE id = sqlc.arg(id) AND school_id = sqlc.arg(school_id)
RETURNING id, school_id, kode, nama, kategori, poin_default, is_active, created_at;

-- name: CountStudentViolationsByType :one
SELECT COUNT(*)::bigint
FROM student_violations
WHERE violation_type_id = sqlc.arg(violation_type_id) AND school_id = sqlc.arg(school_id);

-- name: DeleteViolationType :execrows
DELETE FROM violation_types
WHERE id = sqlc.arg(id) AND school_id = sqlc.arg(school_id);

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

-- name: UpdateDisciplineInspection :one
UPDATE discipline_inspections
SET tanggal = sqlc.arg(tanggal),
    jumlah_diperiksa = sqlc.arg(jumlah_diperiksa),
    catatan = sqlc.narg(catatan)
WHERE id = sqlc.arg(id) AND school_id = sqlc.arg(school_id)
RETURNING id, school_id, class_id, inspector_id, tanggal, jumlah_diperiksa, catatan, created_at;

-- name: DeleteDisciplineInspection :execrows
DELETE FROM discipline_inspections
WHERE id = sqlc.arg(id) AND school_id = sqlc.arg(school_id);

-- name: DeleteStudentViolationsByInspection :exec
DELETE FROM student_violations
WHERE inspection_id = sqlc.arg(inspection_id) AND school_id = sqlc.arg(school_id);

-- name: ListViolationsByInspectionIDs :many
SELECT sv.id, sv.school_id, sv.inspection_id, sv.student_id, sv.violation_type_id, sv.poin, sv.keterangan, sv.tindak_lanjut, sv.tanggal, sv.created_at,
       s.nis, s.nama AS student_name, c.nama_kelas,
       vt.kode AS violation_code, vt.nama AS violation_name, vt.kategori AS violation_category
FROM student_violations sv
JOIN students s ON s.id = sv.student_id
LEFT JOIN class_enrollments ce ON ce.student_id = s.id AND ce.status = 'aktif'
LEFT JOIN classes c ON c.id = ce.class_id
JOIN violation_types vt ON vt.id = sv.violation_type_id
WHERE sv.school_id = sqlc.arg(school_id)
  AND sv.inspection_id = ANY(sqlc.arg(inspection_ids)::bigint[])
ORDER BY s.nama, vt.kode;

-- name: RecordStudentViolation :one
INSERT INTO student_violations (school_id, inspection_id, student_id, violation_type_id, poin, keterangan, tindak_lanjut, tanggal)
VALUES (sqlc.arg(school_id), sqlc.narg(inspection_id), sqlc.arg(student_id), sqlc.arg(violation_type_id), sqlc.arg(poin), sqlc.narg(keterangan), sqlc.narg(tindak_lanjut), sqlc.arg(tanggal))
RETURNING id, school_id, inspection_id, student_id, violation_type_id, poin, keterangan, tindak_lanjut, tanggal, created_at;

-- name: GetStudentViolationByID :one
SELECT sv.id, sv.school_id, sv.inspection_id, sv.student_id, sv.violation_type_id, sv.poin, sv.keterangan, sv.tindak_lanjut, sv.tanggal, sv.created_at,
       s.nis, s.nama AS student_name, c.nama_kelas,
       vt.kode AS violation_code, vt.nama AS violation_name, vt.kategori AS violation_category
FROM student_violations sv
JOIN students s ON s.id = sv.student_id
LEFT JOIN class_enrollments ce ON ce.student_id = s.id AND ce.status = 'aktif'
LEFT JOIN classes c ON c.id = ce.class_id
JOIN violation_types vt ON vt.id = sv.violation_type_id
WHERE sv.id = sqlc.arg(id) AND sv.school_id = sqlc.arg(school_id);

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

-- name: UpdateStudentViolation :one
UPDATE student_violations
SET violation_type_id = sqlc.arg(violation_type_id),
    poin = sqlc.arg(poin),
    keterangan = sqlc.narg(keterangan),
    tindak_lanjut = sqlc.narg(tindak_lanjut),
    tanggal = sqlc.arg(tanggal)
WHERE id = sqlc.arg(id) AND school_id = sqlc.arg(school_id)
RETURNING id, school_id, inspection_id, student_id, violation_type_id, poin, keterangan, tindak_lanjut, tanggal, created_at;

-- name: UpdateStudentViolationFollowup :one
UPDATE student_violations
SET tindak_lanjut = sqlc.arg(tindak_lanjut)
WHERE id = sqlc.arg(id) AND school_id = sqlc.arg(school_id)
RETURNING id, school_id, inspection_id, student_id, violation_type_id, poin, keterangan, tindak_lanjut, tanggal, created_at;

-- name: DeleteStudentViolation :execrows
DELETE FROM student_violations
WHERE id = sqlc.arg(id) AND school_id = sqlc.arg(school_id);

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

-- name: ListSchoolStudents :many
SELECT s.id, s.nis, s.nama, s.jenis_kelamin, s.no_hp_ortu, c.id AS class_id, c.nama_kelas, c.tingkat
FROM class_enrollments ce
JOIN students s ON s.id = ce.student_id
JOIN classes c ON c.id = ce.class_id
JOIN academic_years ay ON ay.id = c.academic_year_id
WHERE ay.school_id = sqlc.arg(school_id)
  AND ay.is_active
  AND ce.status = 'aktif'
  AND s.is_active
  AND (sqlc.narg(class_id)::bigint IS NULL OR c.id = sqlc.narg(class_id))
ORDER BY c.tingkat, c.nama_kelas, s.nama;

-- name: StudentInSchool :one
SELECT EXISTS (
    SELECT 1
    FROM class_enrollments ce
    JOIN classes c ON c.id = ce.class_id
    JOIN academic_years ay ON ay.id = c.academic_year_id
    WHERE ce.student_id = sqlc.arg(student_id)
      AND ay.school_id = sqlc.arg(school_id)
)::boolean;

-- name: ListStudentsWatchlist :many
SELECT s.id AS student_id, s.nis, s.nama AS student_name, s.no_hp_ortu,
       c.id AS class_id, c.nama_kelas,
       COALESCE(SUM(sv.poin), 0)::bigint AS total_points,
       COUNT(sv.id)::bigint AS total_incidents,
       MAX(sv.tanggal)::date AS last_violation_date
FROM student_violations sv
JOIN students s ON s.id = sv.student_id
LEFT JOIN class_enrollments ce ON ce.student_id = s.id AND ce.status = 'aktif'
LEFT JOIN classes c ON c.id = ce.class_id
WHERE sv.school_id = sqlc.arg(school_id)
GROUP BY s.id, s.nis, s.nama, s.no_hp_ortu, c.id, c.nama_kelas
HAVING COALESCE(SUM(sv.poin), 0) >= sqlc.arg(min_points)::bigint
ORDER BY total_points DESC, total_incidents DESC, s.nama;

-- name: ListCounselingRecords :many
SELECT cr.id, cr.school_id, cr.student_id, cr.counselor_id, cr.tanggal, cr.topik, cr.catatan, cr.tindak_lanjut, cr.status, cr.created_at,
       s.nis, s.nama AS student_name, c.nama_kelas, st.nama AS counselor_name
FROM counseling_records cr
JOIN students s ON s.id = cr.student_id
LEFT JOIN class_enrollments ce ON ce.student_id = s.id AND ce.status = 'aktif'
LEFT JOIN classes c ON c.id = ce.class_id
LEFT JOIN staff st ON st.id = cr.counselor_id
WHERE cr.school_id = sqlc.arg(school_id)
  AND (sqlc.narg(student_id)::bigint IS NULL OR cr.student_id = sqlc.narg(student_id))
  AND (sqlc.narg(status)::varchar IS NULL OR cr.status = sqlc.narg(status))
ORDER BY cr.tanggal DESC, cr.created_at DESC;

-- name: GetCounselingRecord :one
SELECT cr.id, cr.school_id, cr.student_id, cr.counselor_id, cr.tanggal, cr.topik, cr.catatan, cr.tindak_lanjut, cr.status, cr.created_at,
       s.nis, s.nama AS student_name, c.nama_kelas, st.nama AS counselor_name
FROM counseling_records cr
JOIN students s ON s.id = cr.student_id
LEFT JOIN class_enrollments ce ON ce.student_id = s.id AND ce.status = 'aktif'
LEFT JOIN classes c ON c.id = ce.class_id
LEFT JOIN staff st ON st.id = cr.counselor_id
WHERE cr.id = sqlc.arg(id) AND cr.school_id = sqlc.arg(school_id);

-- name: CreateCounselingRecord :one
INSERT INTO counseling_records (school_id, student_id, counselor_id, tanggal, topik, catatan, tindak_lanjut, status)
VALUES (sqlc.arg(school_id), sqlc.arg(student_id), sqlc.narg(counselor_id), sqlc.arg(tanggal), sqlc.arg(topik), sqlc.narg(catatan), sqlc.narg(tindak_lanjut), sqlc.arg(status))
RETURNING id;

-- name: UpdateCounselingRecord :one
UPDATE counseling_records
SET tanggal = sqlc.arg(tanggal),
    topik = sqlc.arg(topik),
    catatan = sqlc.narg(catatan),
    tindak_lanjut = sqlc.narg(tindak_lanjut),
    status = sqlc.arg(status)
WHERE id = sqlc.arg(id) AND school_id = sqlc.arg(school_id)
RETURNING id;

-- name: DeleteCounselingRecord :execrows
DELETE FROM counseling_records
WHERE id = sqlc.arg(id) AND school_id = sqlc.arg(school_id);

-- name: ListParentSummons :many
SELECT ps.id, ps.school_id, ps.student_id, ps.created_by, ps.nomor_surat, ps.tanggal_surat, ps.jadwal_pertemuan,
       ps.ruangan, ps.alasan, ps.nama_ortu, ps.no_hp_ortu, ps.status, ps.sent_at, ps.completed_at, ps.catatan_hasil, ps.created_at,
       s.nis, s.nama AS student_name, c.nama_kelas, st.nama AS created_by_name,
       COALESCE((SELECT SUM(sv.poin) FROM student_violations sv WHERE sv.student_id = ps.student_id AND sv.school_id = ps.school_id), 0)::bigint AS total_points
FROM parent_summons ps
JOIN students s ON s.id = ps.student_id
LEFT JOIN class_enrollments ce ON ce.student_id = s.id AND ce.status = 'aktif'
LEFT JOIN classes c ON c.id = ce.class_id
LEFT JOIN staff st ON st.id = ps.created_by
WHERE ps.school_id = sqlc.arg(school_id)
  AND (sqlc.narg(student_id)::bigint IS NULL OR ps.student_id = sqlc.narg(student_id))
  AND (sqlc.narg(status)::varchar IS NULL OR ps.status = sqlc.narg(status))
ORDER BY ps.tanggal_surat DESC, ps.created_at DESC;

-- name: GetParentSummons :one
SELECT ps.id, ps.school_id, ps.student_id, ps.created_by, ps.nomor_surat, ps.tanggal_surat, ps.jadwal_pertemuan,
       ps.ruangan, ps.alasan, ps.nama_ortu, ps.no_hp_ortu, ps.status, ps.sent_at, ps.completed_at, ps.catatan_hasil, ps.created_at,
       s.nis, s.nama AS student_name, c.nama_kelas, st.nama AS created_by_name,
       COALESCE((SELECT SUM(sv.poin) FROM student_violations sv WHERE sv.student_id = ps.student_id AND sv.school_id = ps.school_id), 0)::bigint AS total_points
FROM parent_summons ps
JOIN students s ON s.id = ps.student_id
LEFT JOIN class_enrollments ce ON ce.student_id = s.id AND ce.status = 'aktif'
LEFT JOIN classes c ON c.id = ce.class_id
LEFT JOIN staff st ON st.id = ps.created_by
WHERE ps.id = sqlc.arg(id) AND ps.school_id = sqlc.arg(school_id);

-- name: CreateParentSummons :one
INSERT INTO parent_summons (school_id, student_id, created_by, nomor_surat, tanggal_surat, jadwal_pertemuan, ruangan, alasan, nama_ortu, no_hp_ortu, status)
VALUES (sqlc.arg(school_id), sqlc.arg(student_id), sqlc.narg(created_by), sqlc.arg(nomor_surat), sqlc.arg(tanggal_surat), sqlc.arg(jadwal_pertemuan), sqlc.arg(ruangan), sqlc.arg(alasan), sqlc.narg(nama_ortu), sqlc.narg(no_hp_ortu), sqlc.arg(status))
RETURNING id;

-- name: UpdateParentSummons :one
UPDATE parent_summons
SET nomor_surat = sqlc.arg(nomor_surat),
    tanggal_surat = sqlc.arg(tanggal_surat),
    jadwal_pertemuan = sqlc.arg(jadwal_pertemuan),
    ruangan = sqlc.arg(ruangan),
    alasan = sqlc.arg(alasan),
    nama_ortu = sqlc.narg(nama_ortu),
    no_hp_ortu = sqlc.narg(no_hp_ortu),
    catatan_hasil = sqlc.narg(catatan_hasil)
WHERE id = sqlc.arg(id) AND school_id = sqlc.arg(school_id)
RETURNING id;

-- name: UpdateParentSummonsStatus :one
UPDATE parent_summons
SET status = sqlc.arg(status),
    sent_at = CASE WHEN sqlc.arg(status)::varchar = 'sent' AND sent_at IS NULL THEN now() ELSE sent_at END,
    completed_at = CASE WHEN sqlc.arg(status)::varchar = 'completed' THEN now() ELSE completed_at END,
    catatan_hasil = COALESCE(sqlc.narg(catatan_hasil), catatan_hasil)
WHERE id = sqlc.arg(id) AND school_id = sqlc.arg(school_id)
RETURNING id;

-- name: DeleteParentSummons :execrows
DELETE FROM parent_summons
WHERE id = sqlc.arg(id) AND school_id = sqlc.arg(school_id);
