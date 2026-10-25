DROP TABLE IF EXISTS parent_students;

DELETE FROM user_roles WHERE role_code = 'parent';
DELETE FROM roles WHERE code = 'parent';
