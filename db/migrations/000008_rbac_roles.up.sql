-- Beyond-MVP RBAC wave: add two new role codes.
-- Mirrors the seed style of 000002_identity. Existing roles are left untouched.
INSERT INTO roles (code, name, description) VALUES
  ('staff_tu', 'Staf Tata Usaha', 'Mengelola data master, import/export, dan membaca laporan. Tidak mencatat absensi/kedisiplinan.'),
  ('student', 'Siswa', 'Melihat data kehadiran, kedisiplinan, dan dispensasi miliknya sendiri melalui endpoint /my/*.')
ON CONFLICT (code) DO NOTHING;
