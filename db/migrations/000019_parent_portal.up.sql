-- parent-portal (beyond-MVP wave): a parent/guardian login reads the
-- attendance, discipline and dispensations of the children linked to it.
-- Mirrors the role-seed style of 000008_rbac_roles.
INSERT INTO roles (code, name, description) VALUES
  ('parent', 'Orang Tua / Wali', 'Melihat kehadiran, kedisiplinan, dan dispensasi anak yang tertaut ke akunnya melalui endpoint /parent/*.')
ON CONFLICT (code) DO NOTHING;

-- One login can be linked to several children (siblings), and one student can
-- have several guardian logins (mother/father/guardian). The composite primary
-- key makes re-linking idempotent.
CREATE TABLE parent_students (
  user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  student_id   uuid NOT NULL REFERENCES students(id) ON DELETE CASCADE,
  relationship varchar(30) CHECK (relationship IN ('father','mother','guardian')),
  created_at   timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, student_id)
);
CREATE INDEX parent_students_student_idx ON parent_students (student_id);
