-- One row per login attempt: the stored identity plus the roles read from the
-- database, so a client can never claim a role it was not granted.
-- name: FindUserForLogin :one
SELECT u.id,
       u.school_id,
       u.username,
       u.password_hash,
       u.is_active,
       COALESCE(s.nama, '') AS nama,
       COALESCE(array_agg(r.kode ORDER BY r.id) FILTER (WHERE r.kode IS NOT NULL),
                ARRAY[]::varchar[])::varchar[] AS roles
FROM users u
LEFT JOIN staff s ON s.user_id = u.id
LEFT JOIN user_roles ur ON ur.user_id = u.id
LEFT JOIN roles r ON r.id = ur.role_id
WHERE u.username = sqlc.arg(username)
GROUP BY u.id, u.school_id, u.username, u.password_hash, u.is_active, s.nama;

-- One statement so a failed insert anywhere leaves no half-registered school.
-- name: RegisterSchoolAdmin :one
WITH new_school AS (
    INSERT INTO schools (npsn, nama, jenjang)
    -- Fail the whole statement when the admin role is absent, so the token can
    -- never claim a role the user was not granted.
    SELECT sqlc.arg(npsn), sqlc.arg(school_name), sqlc.arg(jenjang)
    WHERE EXISTS (SELECT 1 FROM roles WHERE kode = 'admin')
    RETURNING id
), new_user AS (
    INSERT INTO users (school_id, username, password_hash)
    SELECT new_school.id, sqlc.arg(username), sqlc.arg(password_hash)
    FROM new_school
    RETURNING id, school_id, username
), new_staff AS (
    INSERT INTO staff (user_id, nama, no_hp, jabatan)
    SELECT new_user.id, sqlc.arg(nama), sqlc.narg(no_hp), 'Administrator'
    FROM new_user
), new_role AS (
    INSERT INTO user_roles (user_id, role_id)
    SELECT new_user.id, r.id
    FROM new_user
    JOIN roles r ON r.kode = 'admin'
)
SELECT id, school_id, username FROM new_user;
