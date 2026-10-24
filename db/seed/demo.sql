-- Local demo data: one guru, one TU user, one class with three students.
-- Not a migration: production databases should not get fake pupils.
-- Apply with: docker compose exec -T postgres psql -U app -d app < db/seed/demo.sql
-- Safe to re-run.

INSERT INTO users (id, username, password_hash) OVERRIDING SYSTEM VALUE VALUES
    (1, 'guru1', '!'),
    (2, 'tu1',   '!')
ON CONFLICT (id) DO NOTHING;

INSERT INTO user_roles (user_id, role_id) VALUES
    (1, (SELECT id FROM roles WHERE kode = 'guru')),
    (2, (SELECT id FROM roles WHERE kode = 'tu'))
ON CONFLICT (user_id, role_id) DO NOTHING;

INSERT INTO staff (id, user_id, nip, nama, jabatan) OVERRIDING SYSTEM VALUE VALUES
    (1, 1, '19800101', 'Ibu Sari',  'Guru Kelas 1A'),
    (2, 2, '19800202', 'Bapak Andi', 'Tata Usaha')
ON CONFLICT (id) DO NOTHING;

INSERT INTO academic_years (id, nama, semester, tgl_mulai, tgl_selesai, is_active) OVERRIDING SYSTEM VALUE VALUES
    (1, '2026/2027', 'ganjil', DATE '2026-07-01', DATE '2026-12-31', TRUE)
ON CONFLICT (id) DO NOTHING;

INSERT INTO classes (id, academic_year_id, wali_kelas_id, nama_kelas, tingkat) OVERRIDING SYSTEM VALUE VALUES
    (1, 1, 1, '1A', 1)
ON CONFLICT (id) DO NOTHING;

INSERT INTO subjects (id, kode, nama) OVERRIDING SYSTEM VALUE VALUES
    (1, 'MTK', 'Matematika')
ON CONFLICT (id) DO NOTHING;

INSERT INTO class_subjects (id, class_id, subject_id, teacher_id) OVERRIDING SYSTEM VALUE VALUES
    (1, 1, 1, 1)
ON CONFLICT (id) DO NOTHING;

INSERT INTO students (id, nis, nama, jenis_kelamin) OVERRIDING SYSTEM VALUE VALUES
    (1, '0001', 'Aisyah Putri',  'P'),
    (2, '0002', 'Budi Santoso',  'L'),
    (3, '0003', 'Citra Lestari', 'P')
ON CONFLICT (id) DO NOTHING;

INSERT INTO class_enrollments (id, class_id, student_id) OVERRIDING SYSTEM VALUE VALUES
    (1, 1, 1),
    (2, 1, 2),
    (3, 1, 3)
ON CONFLICT (id) DO NOTHING;

SELECT setval(pg_get_serial_sequence('users', 'id'), (SELECT max(id) FROM users));
SELECT setval(pg_get_serial_sequence('staff', 'id'), (SELECT max(id) FROM staff));
SELECT setval(pg_get_serial_sequence('academic_years', 'id'), (SELECT max(id) FROM academic_years));
SELECT setval(pg_get_serial_sequence('classes', 'id'), (SELECT max(id) FROM classes));
SELECT setval(pg_get_serial_sequence('subjects', 'id'), (SELECT max(id) FROM subjects));
SELECT setval(pg_get_serial_sequence('class_subjects', 'id'), (SELECT max(id) FROM class_subjects));
SELECT setval(pg_get_serial_sequence('students', 'id'), (SELECT max(id) FROM students));
SELECT setval(pg_get_serial_sequence('class_enrollments', 'id'), (SELECT max(id) FROM class_enrollments));
