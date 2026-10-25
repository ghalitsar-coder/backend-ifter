-- Per-subject attendance (F8, beyond-MVP; PRD §15 names attendance_sessions as the
-- extension point). Daily attendance stays the source of truth for the school day;
-- these tables add lessons: a subject taught to a class by a teacher, the sessions
-- of that lesson, and one status per enrolled student per session.

CREATE TABLE subjects (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code        varchar(30) NOT NULL UNIQUE CHECK (btrim(code) <> ''),
  name        varchar(150) NOT NULL CHECK (btrim(name) <> ''),
  description text,
  is_active   boolean NOT NULL DEFAULT true,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

-- One teacher teaches one subject to one class. is_active retires an assignment
-- without deleting it, so past sessions keep their context.
CREATE TABLE teaching_assignments (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  teacher_id  uuid NOT NULL REFERENCES teachers(id),
  subject_id  uuid NOT NULL REFERENCES subjects(id),
  class_id    uuid NOT NULL REFERENCES classes(id),
  is_active   boolean NOT NULL DEFAULT true,
  created_by  uuid NOT NULL REFERENCES users(id),
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT teaching_assignments_triple_uq UNIQUE (teacher_id, subject_id, class_id)
);
CREATE INDEX teaching_assignments_class_idx ON teaching_assignments (class_id, subject_id);
CREATE INDEX teaching_assignments_teacher_idx ON teaching_assignments (teacher_id);

-- One lesson meeting. period_start/period_end are class-hour (JP) numbers; the
-- start period plus the date identifies the meeting, so two sessions of the same
-- lesson cannot start on the same JP twice.
CREATE TABLE attendance_sessions (
  id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  teaching_assignment_id  uuid NOT NULL REFERENCES teaching_assignments(id),
  session_date            date NOT NULL,
  period_start            smallint NOT NULL CHECK (period_start BETWEEN 1 AND 20),
  period_end              smallint NOT NULL CHECK (period_end BETWEEN period_start AND 20),
  topic                   varchar(200),
  notes                   text,
  status                  varchar(20) NOT NULL DEFAULT 'open' CHECK (status IN ('open','closed')),
  created_by              uuid NOT NULL REFERENCES users(id),
  closed_by               uuid REFERENCES users(id),
  closed_at               timestamptz,
  created_at              timestamptz NOT NULL DEFAULT now(),
  updated_at              timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT attendance_sessions_assignment_day_period_uq
    UNIQUE (teaching_assignment_id, session_date, period_start),
  CONSTRAINT attendance_sessions_closed_pair CHECK (
    (closed_by IS NULL) = (closed_at IS NULL)
    AND (status = 'closed') = (closed_at IS NOT NULL)
  )
);
CREATE INDEX attendance_sessions_date_idx ON attendance_sessions (session_date, teaching_assignment_id);

CREATE TABLE attendance_session_records (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  session_id    uuid NOT NULL REFERENCES attendance_sessions(id) ON DELETE CASCADE,
  student_id    uuid NOT NULL REFERENCES students(id),
  enrollment_id uuid NOT NULL REFERENCES class_enrollments(id),
  status        varchar(20) NOT NULL CHECK (status IN ('present','late','sick','excused','absent')),
  note          text,
  recorded_by   uuid NOT NULL REFERENCES users(id),
  updated_by    uuid REFERENCES users(id),
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT attendance_session_records_student_uq UNIQUE (session_id, student_id)
);
CREATE INDEX attendance_session_records_student_idx
  ON attendance_session_records (student_id, session_id);
CREATE INDEX attendance_session_records_enrollment_idx
  ON attendance_session_records (enrollment_id);

-- The service refuses writes on a closed session; the trigger makes that rule
-- hold for every writer, mirroring the daily closed-day protection in 000005.
CREATE FUNCTION prevent_closed_session_record() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  session_status varchar(20);
BEGIN
  SELECT status INTO session_status
  FROM attendance_sessions
  WHERE id = NEW.session_id;

  IF session_status = 'closed' THEN
    RAISE EXCEPTION 'attendance session is closed';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER attendance_session_records_open_insert
BEFORE INSERT ON attendance_session_records
FOR EACH ROW EXECUTE FUNCTION prevent_closed_session_record();

CREATE TRIGGER attendance_session_records_open_update
BEFORE UPDATE OF status, note ON attendance_session_records
FOR EACH ROW EXECUTE FUNCTION prevent_closed_session_record();

CREATE FUNCTION prevent_session_record_delete() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'attendance session records are append-only';
END;
$$;

CREATE TRIGGER attendance_session_records_no_delete
BEFORE DELETE ON attendance_session_records
FOR EACH ROW EXECUTE FUNCTION prevent_session_record_delete();
