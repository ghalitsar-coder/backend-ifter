-- name: GenerateStudentAttendanceRecaps :execrows
INSERT INTO student_attendance_recaps (
    student_id,
    class_id,
    bulan,
    tahun,
    total_hadir,
    total_sakit,
    total_izin,
    total_alpa,
    persentase_hadir,
    status_kunci
)
SELECT
    sa.student_id,
    s.class_id,
    sqlc.arg(bulan)::smallint,
    sqlc.arg(tahun)::smallint,
    count(*) FILTER (WHERE sa.status = 'hadir')::integer,
    count(*) FILTER (WHERE sa.status = 'sakit')::integer,
    count(*) FILTER (WHERE sa.status = 'izin')::integer,
    count(*) FILTER (WHERE sa.status = 'alpa')::integer,
    round(100.0 * count(*) FILTER (WHERE sa.status = 'hadir') / count(*), 2),
    'draft'
FROM student_attendances sa
JOIN attendance_sessions s ON s.id = sa.session_id
JOIN classes c ON c.id = s.class_id
JOIN academic_years ay ON ay.id = c.academic_year_id
WHERE ay.school_id = sqlc.arg(school_id)
  AND s.tanggal >= make_date(sqlc.arg(tahun)::integer, sqlc.arg(bulan)::integer, 1)
  AND s.tanggal < (make_date(sqlc.arg(tahun)::integer, sqlc.arg(bulan)::integer, 1) + interval '1 month')::date
GROUP BY sa.student_id, s.class_id
ON CONFLICT (student_id, class_id, bulan, tahun)
DO UPDATE SET
    total_hadir = EXCLUDED.total_hadir,
    total_sakit = EXCLUDED.total_sakit,
    total_izin = EXCLUDED.total_izin,
    total_alpa = EXCLUDED.total_alpa,
    persentase_hadir = EXCLUDED.persentase_hadir
WHERE student_attendance_recaps.status_kunci = 'draft';

-- name: ListStudentAttendanceRecaps :many
SELECT
    r.id,
    r.student_id,
    st.nama AS student_name,
    r.class_id,
    c.nama_kelas AS class_name,
    r.bulan,
    r.tahun,
    r.total_hadir,
    r.total_sakit,
    r.total_izin,
    r.total_alpa,
    r.persentase_hadir::float8 AS persentase_hadir,
    r.status_kunci,
    r.verified_by,
    r.created_at
FROM student_attendance_recaps r
JOIN students st ON st.id = r.student_id
JOIN classes c ON c.id = r.class_id
JOIN academic_years ay ON ay.id = c.academic_year_id
WHERE ay.school_id = sqlc.arg(school_id)
  AND r.bulan = sqlc.arg(bulan)
  AND r.tahun = sqlc.arg(tahun)
ORDER BY c.tingkat, c.nama_kelas, st.nama, st.id;

-- name: StudentAttendanceRecapStatus :one
SELECT r.status_kunci
FROM student_attendance_recaps r
JOIN classes c ON c.id = r.class_id
JOIN academic_years ay ON ay.id = c.academic_year_id
WHERE r.id = sqlc.arg(id)
  AND ay.school_id = sqlc.arg(school_id);

-- name: VerifyStudentAttendanceRecap :one
UPDATE student_attendance_recaps r
SET status_kunci = 'dikunci', verified_by = sqlc.arg(user_id)
FROM classes c
JOIN academic_years ay ON ay.id = c.academic_year_id
WHERE r.id = sqlc.arg(id)
  AND r.class_id = c.id
  AND ay.school_id = sqlc.arg(school_id)
  AND r.status_kunci = 'draft'
RETURNING r.id, r.status_kunci, r.verified_by;

-- name: GenerateStaffAttendanceRecaps :execrows
WITH period AS (
    SELECT make_date(sqlc.arg(tahun)::integer, sqlc.arg(bulan)::integer, 1) AS starts_on
), workdays AS (
    SELECT count(*)::integer AS total_hari_kerja
    FROM period p
    CROSS JOIN LATERAL generate_series(
        p.starts_on::timestamp,
        (p.starts_on + interval '1 month' - interval '1 day')::timestamp,
        interval '1 day'
    ) AS calendar(day)
    WHERE extract(isodow FROM calendar.day) < 6
      AND NOT EXISTS (
          SELECT 1
          FROM holidays h
          JOIN academic_years holiday_year ON holiday_year.id = h.academic_year_id
          WHERE h.tanggal = calendar.day::date
            AND holiday_year.school_id = sqlc.arg(school_id)
      )
), scoped_staff AS (
    SELECT st.id, st.is_active
    FROM staff st
    JOIN users u ON u.id = st.user_id
    WHERE u.school_id = sqlc.arg(school_id)
      AND (
          st.is_active
          OR EXISTS (
              SELECT 1
              FROM staff_attendances sa
              CROSS JOIN period p
              WHERE sa.staff_id = st.id
                AND sa.tanggal >= p.starts_on
                AND sa.tanggal < (p.starts_on + interval '1 month')::date
          )
      )
), attendance_totals AS (
    SELECT
        sa.staff_id,
        count(*) FILTER (WHERE sa.status = 'hadir')::integer AS total_hadir,
        count(*) FILTER (WHERE sa.status = 'terlambat')::integer AS total_terlambat,
        count(*) FILTER (WHERE sa.status = 'izin')::integer AS total_izin,
        count(*) FILTER (WHERE sa.status = 'sakit')::integer AS total_sakit,
        count(*) FILTER (WHERE sa.status = 'alpa')::integer AS total_alpa
    FROM staff_attendances sa
    CROSS JOIN period p
    JOIN scoped_staff st ON st.id = sa.staff_id
    WHERE sa.tanggal >= p.starts_on
      AND sa.tanggal < (p.starts_on + interval '1 month')::date
    GROUP BY sa.staff_id
)
INSERT INTO staff_attendance_recaps (
    staff_id, bulan, tahun, total_hari_kerja, total_hadir, total_terlambat,
    total_izin, total_sakit, total_alpa, persentase_hadir, status_kunci
)
SELECT
    st.id,
    sqlc.arg(bulan)::smallint,
    sqlc.arg(tahun)::smallint,
    wd.total_hari_kerja,
    coalesce(at.total_hadir, 0),
    coalesce(at.total_terlambat, 0),
    coalesce(at.total_izin, 0),
    coalesce(at.total_sakit, 0),
    coalesce(at.total_alpa, 0),
    CASE WHEN wd.total_hari_kerja = 0 THEN 0
         ELSE round(100.0 * (coalesce(at.total_hadir, 0) + coalesce(at.total_terlambat, 0)) / wd.total_hari_kerja, 2)
    END,
    'draft'
FROM scoped_staff st
CROSS JOIN workdays wd
LEFT JOIN attendance_totals at ON at.staff_id = st.id
ON CONFLICT (staff_id, bulan, tahun)
DO UPDATE SET
    total_hari_kerja = EXCLUDED.total_hari_kerja,
    total_hadir = EXCLUDED.total_hadir,
    total_terlambat = EXCLUDED.total_terlambat,
    total_izin = EXCLUDED.total_izin,
    total_sakit = EXCLUDED.total_sakit,
    total_alpa = EXCLUDED.total_alpa,
    persentase_hadir = EXCLUDED.persentase_hadir
WHERE staff_attendance_recaps.status_kunci = 'draft';

-- name: ListStaffAttendanceRecaps :many
SELECT
    r.id,
    r.staff_id,
    st.nama AS staff_name,
    st.nip,
    r.bulan,
    r.tahun,
    r.total_hari_kerja,
    r.total_hadir,
    r.total_terlambat,
    r.total_izin,
    r.total_sakit,
    r.total_alpa,
    r.persentase_hadir::float8 AS persentase_hadir,
    r.status_kunci,
    r.verified_by,
    r.created_at
FROM staff_attendance_recaps r
JOIN staff st ON st.id = r.staff_id
JOIN users u ON u.id = st.user_id
WHERE u.school_id = sqlc.arg(school_id)
  AND r.bulan = sqlc.arg(bulan)
  AND r.tahun = sqlc.arg(tahun)
ORDER BY st.nama, st.id;

-- name: StaffAttendanceRecapStatus :one
SELECT r.status_kunci
FROM staff_attendance_recaps r
JOIN staff st ON st.id = r.staff_id
JOIN users u ON u.id = st.user_id
WHERE r.id = sqlc.arg(id)
  AND u.school_id = sqlc.arg(school_id);

-- name: VerifyStaffAttendanceRecap :one
UPDATE staff_attendance_recaps r
SET status_kunci = 'dikunci', verified_by = sqlc.arg(user_id)
FROM staff st
JOIN users u ON u.id = st.user_id
WHERE r.id = sqlc.arg(id)
  AND r.staff_id = st.id
  AND u.school_id = sqlc.arg(school_id)
  AND r.status_kunci = 'draft'
RETURNING r.id, r.status_kunci, r.verified_by;