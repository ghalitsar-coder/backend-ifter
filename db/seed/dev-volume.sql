-- Optional volume fixtures for local UI/API testing: one dedicated tenant with
-- six classes, 180 students and two weeks of daily attendance.
-- Apply with:
--   docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U app -d app < db/seed/dev-volume.sql
-- Safe to re-run: every insert is keyed on a natural key and never updates an
-- existing row. NPSN 00000003 is reserved for this file.
--
-- Accounts: volume_admin / guru123 (admin) and volume_guru / guru123 (guru,
-- wali kelas and subject teacher for 7A only).

BEGIN;

INSERT INTO schools (npsn, nama, jenjang)
VALUES ('00000003', 'SMP Volume Demo', 'SMP')
ON CONFLICT (npsn) DO NOTHING;

INSERT INTO academic_years (school_id, nama, semester, tgl_mulai, tgl_selesai, is_active)
SELECT id, '2026/2027', 'ganjil', DATE '2026-07-01', DATE '2026-12-31', TRUE
FROM schools
WHERE npsn = '00000003'
ON CONFLICT (school_id, nama, semester) DO NOTHING;

INSERT INTO users (school_id, username, password_hash)
SELECT s.id, v.username, '$2a$10$90sFxvRpAdKLNBpBGzGVB.Oit7/VRY/ooWrKiL2OlwrBxpCPvb0y2'
FROM schools s
CROSS JOIN (VALUES ('volume_admin'), ('volume_guru')) AS v(username)
WHERE s.npsn = '00000003'
ON CONFLICT (username) DO NOTHING;

INSERT INTO user_roles (user_id, role_id)
SELECT u.id, r.id
FROM users u
JOIN schools s ON s.id = u.school_id
JOIN (VALUES ('volume_admin', 'admin'), ('volume_guru', 'guru')) AS v(username, role)
    ON v.username = u.username
JOIN roles r ON r.kode = v.role
WHERE s.npsn = '00000003'
ON CONFLICT (user_id, role_id) DO NOTHING;

INSERT INTO staff (user_id, nip, nama, jabatan)
SELECT u.id, v.nip, v.nama, v.jabatan
FROM users u
JOIN schools s ON s.id = u.school_id
JOIN (VALUES
    ('volume_admin', '9090001', 'Admin Volume', 'Administrator'),
    ('volume_guru', '9090002', 'Guru Volume', 'Guru Kelas')
) AS v(username, nip, nama, jabatan) ON v.username = u.username
WHERE s.npsn = '00000003'
ON CONFLICT (user_id) DO NOTHING;

-- Only 7A gets a wali kelas and subject assignments, so a non-manage guru
-- hits 403 on every other class.
INSERT INTO classes (academic_year_id, wali_kelas_id, nama_kelas, tingkat)
SELECT ay.id,
       CASE WHEN v.nama_kelas = '7A' THEN (SELECT id FROM staff WHERE nip = '9090002') END,
       v.nama_kelas,
       v.tingkat
FROM academic_years ay
JOIN schools s ON s.id = ay.school_id
CROSS JOIN (VALUES
    ('7A', 1), ('7B', 2), ('8A', 3), ('8B', 4), ('9A', 5), ('9B', 6)
) AS v(nama_kelas, tingkat)
WHERE s.npsn = '00000003' AND ay.nama = '2026/2027' AND ay.semester = 'ganjil'
ON CONFLICT (academic_year_id, nama_kelas) DO NOTHING;

INSERT INTO students (nis, nisn, nama, jenis_kelamin, no_hp_ortu)
SELECT (3000 + i)::text,
       (4000000 + i)::text,
       'Siswa Volume ' || i,
       CASE WHEN i % 2 = 0 THEN 'L' ELSE 'P' END,
       '0812000' || lpad(i::text, 4, '0')
FROM generate_series(1, 180) AS i
ON CONFLICT (nis) DO NOTHING;

-- 30 students per class, ordered so the mapping is stable across runs.
WITH kelas AS (
    SELECT c.id, row_number() OVER (ORDER BY c.nama_kelas) AS rn
    FROM classes c
    JOIN academic_years ay ON ay.id = c.academic_year_id
    JOIN schools s ON s.id = ay.school_id
    WHERE s.npsn = '00000003'
), siswa AS (
    SELECT st.id, row_number() OVER (ORDER BY st.nis) AS rn
    FROM students st
    WHERE st.nis BETWEEN '3001' AND '3180'
)
INSERT INTO class_enrollments (class_id, student_id)
SELECT kelas.id, siswa.id
FROM siswa
JOIN kelas ON kelas.rn = ((siswa.rn - 1) / 30) + 1
ON CONFLICT DO NOTHING;

INSERT INTO subjects (kode, nama) VALUES
    ('MTK', 'Matematika'),
    ('BIN', 'Bahasa Indonesia'),
    ('IPA', 'Ilmu Pengetahuan Alam'),
    ('IPS', 'Ilmu Pengetahuan Sosial'),
    ('ENG', 'Bahasa Inggris'),
    ('PJOK', 'Pendidikan Jasmani')
ON CONFLICT (kode) DO NOTHING;

INSERT INTO class_subjects (class_id, subject_id, teacher_id)
SELECT c.id, sub.id, g.id
FROM classes c
JOIN academic_years ay ON ay.id = c.academic_year_id
JOIN schools s ON s.id = ay.school_id
JOIN subjects sub ON sub.kode IN ('MTK', 'BIN')
JOIN staff g ON g.nip = '9090002'
WHERE s.npsn = '00000003' AND c.nama_kelas = '7A'
ON CONFLICT (class_id, subject_id) DO NOTHING;

-- Ten weekdays in July 2026, one daily session per class.
WITH hari AS (
    SELECT d::date AS tanggal
    FROM generate_series(DATE '2026-07-01', DATE '2026-07-14', INTERVAL '1 day') AS d
    WHERE EXTRACT(ISODOW FROM d) < 6
)
INSERT INTO attendance_sessions (class_id, recorded_by, tanggal)
SELECT c.id, g.id, hari.tanggal
FROM classes c
JOIN academic_years ay ON ay.id = c.academic_year_id
JOIN schools s ON s.id = ay.school_id
JOIN staff g ON g.nip = '9090002'
CROSS JOIN hari
WHERE s.npsn = '00000003'
ON CONFLICT DO NOTHING;

INSERT INTO student_attendances (session_id, student_id, status)
SELECT ses.id,
       ce.student_id,
       CASE (ce.student_id % 17)
           WHEN 0 THEN 'alpa'
           WHEN 1 THEN 'sakit'
           WHEN 2 THEN 'izin'
           ELSE 'hadir'
       END
FROM attendance_sessions ses
JOIN classes c ON c.id = ses.class_id
JOIN academic_years ay ON ay.id = c.academic_year_id
JOIN schools s ON s.id = ay.school_id
JOIN class_enrollments ce ON ce.class_id = c.id AND ce.status = 'aktif'
WHERE s.npsn = '00000003'
ON CONFLICT (session_id, student_id) DO NOTHING;

WITH hari AS (
    SELECT d::date AS tanggal
    FROM generate_series(DATE '2026-07-01', DATE '2026-07-14', INTERVAL '1 day') AS d
    WHERE EXTRACT(ISODOW FROM d) < 6
)
INSERT INTO staff_attendances (staff_id, tanggal, jam_masuk, jam_pulang, status, metode)
SELECT g.id,
       hari.tanggal,
       hari.tanggal + TIME '07:00',
       hari.tanggal + TIME '14:00',
       CASE WHEN EXTRACT(ISODOW FROM hari.tanggal) = 1 THEN 'terlambat' ELSE 'hadir' END,
       'manual'
FROM staff g
JOIN users u ON u.id = g.user_id
JOIN schools s ON s.id = u.school_id
CROSS JOIN hari
WHERE s.npsn = '00000003'
ON CONFLICT (staff_id, tanggal) DO NOTHING;

SELECT
    (SELECT count(*) FROM users u JOIN schools s ON s.id = u.school_id
      WHERE s.npsn = '00000003') AS users,
    (SELECT count(*) FROM classes c JOIN academic_years ay ON ay.id = c.academic_year_id
      JOIN schools s ON s.id = ay.school_id WHERE s.npsn = '00000003') AS classes,
    (SELECT count(*) FROM students WHERE nis BETWEEN '3001' AND '3180') AS students,
    (SELECT count(*) FROM class_enrollments ce JOIN classes c ON c.id = ce.class_id
      JOIN academic_years ay ON ay.id = c.academic_year_id
      JOIN schools s ON s.id = ay.school_id WHERE s.npsn = '00000003') AS enrollments,
    (SELECT count(*) FROM attendance_sessions ses JOIN classes c ON c.id = ses.class_id
      JOIN academic_years ay ON ay.id = c.academic_year_id
      JOIN schools s ON s.id = ay.school_id WHERE s.npsn = '00000003') AS sessions,
    (SELECT count(*) FROM student_attendances sa
      JOIN attendance_sessions ses ON ses.id = sa.session_id
      JOIN classes c ON c.id = ses.class_id
      JOIN academic_years ay ON ay.id = c.academic_year_id
      JOIN schools s ON s.id = ay.school_id WHERE s.npsn = '00000003') AS attendances;

COMMIT;
