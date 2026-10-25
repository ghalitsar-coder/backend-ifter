-- Employment-type package (docs/API_ADDENDUM.md §5/§6.5 + package spec, migration 000013).
-- Master data of teacher employment types (code-keyed, admin + staff_tu maintain
-- it). The package spec's three seeded codes are the contract categories the
-- dashboard breakdown counts by (permanent / honorary / staff_tu); a teacher
-- without an employment_type_id is "unknown" (the spec's NULL = unknown
-- backfill, expressed here as the FK staying NULL for every existing row).
CREATE TABLE employment_types (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code        varchar(30) NOT NULL UNIQUE,
  name        varchar(100) NOT NULL,
  description text,
  is_active   boolean NOT NULL DEFAULT true,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT employment_types_code_not_blank CHECK (code = btrim(code) AND code <> ''),
  CONSTRAINT employment_types_code_format CHECK (code ~ '^[a-z0-9_]+$'),
  CONSTRAINT employment_types_name_not_blank CHECK (btrim(name) <> ''),
  CONSTRAINT employment_types_description_max CHECK (description IS NULL OR char_length(description) <= 2000)
);

-- Seed the three contract categories. Seeding style mirrors 000002_identity /
-- the sanctioned ON CONFLICT (code) DO NOTHING upsert.
INSERT INTO employment_types (code, name, description) VALUES
  ('permanent', 'Permanen (PNS/PPPK)', 'Guru dengan status kepegawaian permanen'),
  ('honorary',  'Honorer',             'Guru dengan status kepegawaian honorer'),
  ('staff_tu',  'Staf TU',             'Staf tata usaha')
ON CONFLICT (code) DO NOTHING;

-- teachers.employment_type_id: nullable FK; NULL keeps the spec's
-- "backfill NULL = unknown" semantics for every pre-existing teacher row.
ALTER TABLE teachers
  ADD COLUMN employment_type_id uuid NULL REFERENCES employment_types(id);

CREATE INDEX teachers_employment_type_idx
  ON teachers (employment_type_id)
  WHERE employment_type_id IS NOT NULL;
