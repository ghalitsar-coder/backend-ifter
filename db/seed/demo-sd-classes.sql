-- Local SD Islam Bakti Asih classes (1A..6B), students, and initial discipline sample data.
-- Not a migration: production databases should not get fake pupils.
-- Safe to re-run (idempotent).

BEGIN;

-- Update parent phone numbers for existing demo students in 1A if missing.
UPDATE students SET no_hp_ortu = '6281210000001' WHERE nis = '0001' AND no_hp_ortu IS NULL;
UPDATE students SET no_hp_ortu = '6281210000002' WHERE nis = '0002' AND no_hp_ortu IS NULL;
UPDATE students SET no_hp_ortu = '6281210000003' WHERE nis = '0003' AND no_hp_ortu IS NULL;

-- Ensure all SD classes (1A..6B) exist for the active academic year of SD Islam Bakti Asih (school_id=1).
INSERT INTO classes (academic_year_id, wali_kelas_id, nama_kelas, tingkat)
SELECT ay.id, 1, c.nama_kelas, c.tingkat
FROM academic_years ay
CROSS JOIN (VALUES
    ('1A', 1), ('1B', 1),
    ('2A', 2), ('2B', 2),
    ('3A', 3), ('3B', 3),
    ('4A', 4), ('4B', 4),
    ('5A', 5), ('5B', 5),
    ('6A', 6), ('6B', 6)
) AS c(nama_kelas, tingkat)
WHERE ay.school_id = 1 AND ay.is_active = TRUE
ON CONFLICT (academic_year_id, nama_kelas) DO NOTHING;

-- Insert sample SD students across 1B..6B.
WITH sample_students(nis, nisn, nama, jenis_kelamin, no_hp_ortu, nama_kelas) AS (
    VALUES
        ('1001', '0161000001', 'Rafa Pratama',      'L', '6281210001001', '1B'),
        ('1002', '0161000002', 'Nabila Zahra',      'P', '6281210001002', '1B'),
        ('1003', '0151000003', 'Daffa Ramadhan',    'L', '6281210001003', '2A'),
        ('1004', '0151000004', 'Keyla Anindita',    'P', '6281210001004', '2A'),
        ('1005', '0151000005', 'Zidan Alfarizi',    'L', '6281210001005', '2B'),
        ('1006', '0151000006', 'Salsa Azzahra',     'P', '6281210001006', '2B'),
        ('1007', '0141000007', 'Fajar Maulana',     'L', '6281210001007', '3A'),
        ('1008', '0141000008', 'Hana Humaira',      'P', '6281210001008', '3A'),
        ('1009', '0141000009', 'Bagas Saputra',     'L', '6281210001009', '3B'),
        ('1010', '0141000010', 'Kirana Dewi',       'P', '6281210001010', '3B'),
        ('1011', '0131000011', 'Aldo Firmansyah',   'L', '6281210001011', '4A'),
        ('1012', '0131000012', 'Nadia Putri',       'P', '6281210001012', '4A'),
        ('1013', '0131000013', 'Rayhan Hakim',      'L', '6281210001013', '4B'),
        ('1014', '0131000014', 'Aulia Rahma',       'P', '6281210001014', '4B'),
        ('1015', '0121000015', 'Rizky Pratama',     'L', '6281210001015', '5A'),
        ('1016', '0121000016', 'Zahira Salsabila',  'P', '6281210001016', '5A'),
        ('1017', '0121000017', 'Dimas Nugraha',     'L', '6281210001017', '5B'),
        ('1018', '0121000018', 'Tiara Permata',     'P', '6281210001018', '5B'),
        ('1019', '0111000019', 'Farhan Akbar',      'L', '6281210001019', '6A'),
        ('1020', '0111000020', 'Syifa Khairunnisa', 'P', '6281210001020', '6A'),
        ('1021', '0111000021', 'Ilham Fauzan',      'L', '6281210001021', '6B'),
        ('1022', '0111000022', 'Rania Kartika',     'P', '6281210001022', '6B')
)
INSERT INTO students (nis, nisn, nama, jenis_kelamin, no_hp_ortu)
SELECT nis, nisn, nama, jenis_kelamin, no_hp_ortu
FROM sample_students
ON CONFLICT (nis) DO UPDATE
SET no_hp_ortu = COALESCE(students.no_hp_ortu, EXCLUDED.no_hp_ortu);

-- Enroll inserted students into their respective SD classes if they do not yet have an active enrollment.
WITH sample_enroll(nis, nama_kelas) AS (
    VALUES
        ('1001', '1B'), ('1002', '1B'),
        ('1003', '2A'), ('1004', '2A'),
        ('1005', '2B'), ('1006', '2B'),
        ('1007', '3A'), ('1008', '3A'),
        ('1009', '3B'), ('1010', '3B'),
        ('1011', '4A'), ('1012', '4A'),
        ('1013', '4B'), ('1014', '4B'),
        ('1015', '5A'), ('1016', '5A'),
        ('1017', '5B'), ('1018', '5B'),
        ('1019', '6A'), ('1020', '6A'),
        ('1021', '6B'), ('1022', '6B')
)
INSERT INTO class_enrollments (class_id, student_id, status)
SELECT c.id, s.id, 'aktif'
FROM sample_enroll se
JOIN students s ON s.nis = se.nis
JOIN classes c ON c.nama_kelas = se.nama_kelas
JOIN academic_years ay ON ay.id = c.academic_year_id AND ay.school_id = 1 AND ay.is_active = TRUE
WHERE NOT EXISTS (
    SELECT 1 FROM class_enrollments ce WHERE ce.student_id = s.id AND ce.status = 'aktif'
)
ON CONFLICT (class_id, student_id) DO NOTHING;

-- Seed initial realistic SD discipline data only when empty for school_id = 1.
INSERT INTO student_violations (school_id, student_id, violation_type_id, poin, keterangan, tindak_lanjut, tanggal)
SELECT 1, s.id, vt.id, v.poin, v.keterangan, v.tindak_lanjut, v.tanggal
FROM (VALUES
    ('1015', 'PRL-05', 50, 'Mendorong teman saat istirahat di halaman sekolah (pembinaan bersama wali kelas 5A).', 'Pemanggilan Orang Tua & SP1', CURRENT_DATE - INTERVAL '3 days'),
    ('1007', 'PRL-04', 15, 'Membawa mainan/jam tangan pintar berkamera ke dalam kelas saat pelajaran.', 'Disita sementara oleh wali kelas 3A', CURRENT_DATE - INTERVAL '2 days'),
    ('1007', 'PRL-03', 5,  'Mengobrol dan mengganggu teman saat kegiatan membaca pagi.', 'Teguran Lisan & Pembinaan', CURRENT_DATE - INTERVAL '1 day'),
    ('1011', 'ATR-01', 3,  'Tidak mengenakan topi dan dasi seragam saat upacara hari Senin.', 'Sudah dicatat pada apel pagi', CURRENT_DATE - INTERVAL '1 day'),
    ('1011', 'KRP-01', 3,  'Rambut melebihi kerah baju seragam sekolah.', 'Diingatkan untuk merapikan rambut', CURRENT_DATE),
    ('0002', 'PRL-01', 2,  'Terlambat datang 10 menit saat jam masuk kelas 1A.', 'Selesai', CURRENT_DATE)
) AS v(nis, kode, poin, keterangan, tindak_lanjut, tanggal)
JOIN students s ON s.nis = v.nis
JOIN violation_types vt ON vt.school_id = 1 AND vt.kode = v.kode
WHERE NOT EXISTS (SELECT 1 FROM student_violations WHERE school_id = 1);

INSERT INTO counseling_records (school_id, student_id, counselor_id, tanggal, topik, catatan, tindak_lanjut, status)
SELECT 1,
       s.id,
       (SELECT id FROM staff WHERE nip = '19800404' LIMIT 1),
       v.tanggal,
       v.topik,
       v.catatan,
       v.tindak_lanjut,
       v.status
FROM (VALUES
    ('1015', CURRENT_DATE - INTERVAL '2 days', 'Pembinaan Pengendalian Emosi & Interaksi Teman Sebaya', 'Siswa bersedia meminta maaf kepada teman dan membuat komitmen perilaku baik di kelas 5A.', 'Pemantauan harian oleh wali kelas selama 2 minggu', 'ongoing'),
    ('1007', CURRENT_DATE - INTERVAL '1 day',  'Edukasi Ketertiban Barang Bawaan Sekolah', 'Orang tua sudah dihubungi terkait aturan tidak membawa perangkat elektronik ke SD.', 'Orang tua mengambil barang di ruang kesiswaan', 'completed')
) AS v(nis, tanggal, topik, catatan, tindak_lanjut, status)
JOIN students s ON s.nis = v.nis
WHERE NOT EXISTS (SELECT 1 FROM counseling_records WHERE school_id = 1);

INSERT INTO parent_summons (school_id, student_id, created_by, nomor_surat, tanggal_surat, jadwal_pertemuan, ruangan, alasan, nama_ortu, no_hp_ortu, status)
SELECT 1,
       s.id,
       (SELECT id FROM staff WHERE nip = '19800404' LIMIT 1),
       v.nomor_surat,
       CURRENT_DATE,
       v.jadwal_pertemuan,
       v.ruangan,
       v.alasan,
       v.nama_ortu,
       s.no_hp_ortu,
       v.status
FROM (VALUES
    ('1015', '001/SD-BA/KESISWAAN/XI/2026', CURRENT_TIMESTAMP + INTERVAL '2 days', 'Ruang Bimbingan Konseling (BK) SD Islam Bakti Asih', 'Akumulasi poin pelanggaran mencapai 50 poin (Perkelahian / Bullying) — Pembinaan bersama orang tua dan penerbitan SP1.', 'Bapak Hendra Pratama', 'draft'),
    ('1007', '002/SD-BA/KESISWAAN/XI/2026', CURRENT_TIMESTAMP + INTERVAL '1 day',  'Ruang Kesiswaan SD Islam Bakti Asih', 'Pembinaan tata tertib membawa barang terlarang di sekolah & klarifikasi wali kelas 3A.', 'Bapak Joko Susilo', 'sent')
) AS v(nis, nomor_surat, jadwal_pertemuan, ruangan, alasan, nama_ortu, status)
JOIN students s ON s.nis = v.nis
WHERE NOT EXISTS (SELECT 1 FROM parent_summons WHERE school_id = 1)
ON CONFLICT (school_id, nomor_surat) DO NOTHING;

SELECT setval(pg_get_serial_sequence('classes', 'id'), COALESCE((SELECT max(id) FROM classes), 1));
SELECT setval(pg_get_serial_sequence('students', 'id'), COALESCE((SELECT max(id) FROM students), 1));
SELECT setval(pg_get_serial_sequence('class_enrollments', 'id'), COALESCE((SELECT max(id) FROM class_enrollments), 1));

COMMIT;
