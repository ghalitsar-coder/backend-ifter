DROP INDEX IF EXISTS students_user_id_lookup_idx;
ALTER TABLE students DROP COLUMN IF EXISTS user_id;
