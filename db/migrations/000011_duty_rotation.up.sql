-- Duty-rotation package (docs/API_ADDENDUM.md §5/§6.3, migration 000011).
-- Substitutions of the duty-assignment master schedule (admin CRUD stays in
-- router.go on groups.Admin; this package owns the substitution surface plus
-- the uniform-note reports that accompany the duty check).
--
-- duty_rotations is the ADDENDUM §5 shape (status/rotation_date) carrying the
-- package spec's substitution semantics: one teacher replacing the assigned
-- duty teacher on a concrete rotation date. The unique triple
-- (assignment, date, substitute) blocks duplicate substitutions per PRD §11.
CREATE TABLE duty_rotations (
  id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  duty_assignment_id    uuid NOT NULL REFERENCES duty_assignments(id),
  substitute_teacher_id uuid NOT NULL REFERENCES teachers(id),
  rotation_date         date NOT NULL,
  status                varchar(20) NOT NULL DEFAULT 'substituted'
                        CHECK (status IN ('substituted','cancelled')),
  reason                text NOT NULL CHECK (btrim(reason) <> ''),
  created_by            uuid NOT NULL REFERENCES users(id),
  created_at            timestamptz NOT NULL DEFAULT now(),
  updated_at            timestamptz NOT NULL DEFAULT now(),
  -- The unique triple stays across cancellation: substitution history is kept
  -- (PRD §11 idempotency), a re-substitution gets a new substitute/date.
  CONSTRAINT duty_rotations_assignment_date_sub_uq
    UNIQUE (duty_assignment_id, rotation_date, substitute_teacher_id),
  CONSTRAINT duty_rotations_reason_max CHECK (char_length(reason) <= 500)
);

CREATE INDEX duty_rotations_date_idx ON duty_rotations (rotation_date);
CREATE INDEX duty_rotations_assignment_idx ON duty_rotations (duty_assignment_id, rotation_date);

-- uniform_reports: catatatan seragam (duty check notes). scope 'teacher' is a
-- statement by/for a teacher; scope 'student' points at one inspected student.
-- A teacher-scope row never carries a student target.
CREATE TABLE uniform_reports (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  for_date          date NOT NULL,
  teacher_id        uuid NOT NULL REFERENCES teachers(id),
  scope             varchar(10) NOT NULL CHECK (scope IN ('student','teacher')),
  target_student_id uuid REFERENCES students(id),
  note              text,
  created_at        timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT uniform_reports_teacher_scope_no_target
    CHECK (scope <> 'teacher' OR target_student_id IS NULL),
  CONSTRAINT uniform_reports_note_max CHECK (note IS NULL OR char_length(note) <= 500)
);

CREATE INDEX uniform_reports_date_idx ON uniform_reports (for_date, teacher_id);
