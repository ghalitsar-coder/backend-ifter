-- Extra local demo accounts for the read-only roles in the main demo school.
-- Not a migration: production databases should not get fake staff.
-- Apply with: docker compose exec -T postgres psql -U app -d app < db/seed/demo-roles.sql
-- Safe to re-run. All accounts log in with the password "guru123".

INSERT INTO users (school_id, username, password_hash)
SELECT s.id, v.username, '$2a$10$90sFxvRpAdKLNBpBGzGVB.Oit7/VRY/ooWrKiL2OlwrBxpCPvb0y2'
FROM schools s
CROSS JOIN (VALUES ('kepsek1'), ('kesiswaan1')) AS v(username)
WHERE s.npsn = '00000000'
ON CONFLICT (username) DO NOTHING;

INSERT INTO user_roles (user_id, role_id)
SELECT u.id, r.id
FROM (VALUES
    ('kepsek1',     'kepala_sekolah'),
    ('kesiswaan1',  'kesiswaan')
) AS v(username, role_kode)
JOIN users u ON u.username = v.username
JOIN roles r ON r.kode = v.role_kode
ON CONFLICT (user_id, role_id) DO NOTHING;

INSERT INTO staff (user_id, nip, nama, jabatan)
SELECT u.id, v.nip, v.nama, v.jabatan
FROM (VALUES
    ('kepsek1',    '19800303', 'Bapak Hendra', 'Kepala Sekolah'),
    ('kesiswaan1', '19800404', 'Ibu Rina',     'Kesiswaan')
) AS v(username, nip, nama, jabatan)
JOIN users u ON u.username = v.username
ON CONFLICT DO NOTHING;

SELECT u.username, r.kode AS role, u.school_id
FROM users u
JOIN user_roles ur ON ur.user_id = u.id
JOIN roles r ON r.id = ur.role_id
WHERE u.username IN ('guru1', 'tu1', 'kepsek1', 'kesiswaan1')
ORDER BY u.username;
