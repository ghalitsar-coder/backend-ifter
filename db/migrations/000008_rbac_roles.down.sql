-- Revert 000008_rbac_roles: remove only the two roles added in this migration.
DELETE FROM user_roles WHERE role_code IN ('staff_tu', 'student');
DELETE FROM roles WHERE code IN ('staff_tu', 'student');
