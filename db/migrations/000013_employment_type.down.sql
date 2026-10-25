DROP INDEX IF EXISTS teachers_employment_type_idx;
ALTER TABLE teachers DROP COLUMN IF EXISTS employment_type_id;
DROP TABLE IF EXISTS employment_types;
