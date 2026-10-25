-- student-notes (FR-C03): free-form administrative notes kept per student.
-- Notes are staff-authored, appended to a student's record and deletable, but
-- never edited in place: the table is an append/delete log ordered newest
-- first. Deleting the student removes the notes with it.
CREATE TABLE student_notes (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  student_id uuid NOT NULL REFERENCES students(id) ON DELETE CASCADE,
  note       text NOT NULL CHECK (btrim(note) <> ''),
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX student_notes_student_created_idx
  ON student_notes (student_id, created_at DESC);
