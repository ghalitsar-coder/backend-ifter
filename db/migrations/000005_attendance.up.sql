CREATE TABLE attendance_records (
  id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  student_id          uuid NOT NULL,
  enrollment_id       uuid NOT NULL,
  attendance_date     date NOT NULL,
  status              varchar(20) NOT NULL
                      CHECK (status IN ('present','late','sick','excused','absent')),
  check_in_at         timestamptz,
  notes               text,
  source              varchar(12) NOT NULL DEFAULT 'manual'
                      CHECK (source IN ('manual','system_close','backfill')),
  recorded_by_user_id uuid NOT NULL REFERENCES users(id),
  updated_by_user_id  uuid NOT NULL REFERENCES users(id),
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT attendance_student_day_uq UNIQUE (student_id, attendance_date),
  CONSTRAINT attendance_enrollment_fk
    FOREIGN KEY (enrollment_id, student_id) REFERENCES class_enrollments (id, student_id),
  CONSTRAINT attendance_checkin_only_if_present
    CHECK (status IN ('present','late') OR check_in_at IS NULL),
  CONSTRAINT attendance_system_close_absent
    CHECK (source <> 'system_close' OR status = 'absent')
);
CREATE INDEX attendance_date_status_idx ON attendance_records (attendance_date, status);
CREATE INDEX attendance_enrollment_idx ON attendance_records (enrollment_id, attendance_date);

CREATE TABLE attendance_corrections (
  id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  attendance_record_id uuid NOT NULL REFERENCES attendance_records(id),
  old_status           varchar(20) NOT NULL,
  new_status           varchar(20) NOT NULL,
  old_check_in_at      timestamptz,
  new_check_in_at      timestamptz,
  old_notes            text,
  new_notes            text,
  reason               text NOT NULL CHECK (btrim(reason) <> ''),
  corrected_by_user_id uuid NOT NULL REFERENCES users(id),
  corrected_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX attendance_corrections_record_idx
  ON attendance_corrections (attendance_record_id, corrected_at);

CREATE TABLE attendance_day_closures (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  class_id          uuid NOT NULL REFERENCES classes(id),
  attendance_date   date NOT NULL,
  close_type        varchar(10) NOT NULL CHECK (close_type IN ('manual','auto')),
  closed_by_user_id uuid NOT NULL REFERENCES users(id),
  closed_at         timestamptz NOT NULL DEFAULT now(),
  absent_filled     integer NOT NULL DEFAULT 0 CHECK (absent_filled >= 0),
  CONSTRAINT closures_class_day_uq UNIQUE (class_id, attendance_date)
);
CREATE INDEX attendance_closures_date_idx ON attendance_day_closures (attendance_date);

CREATE FUNCTION check_attendance_within_enrollment() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  enrollment_start date;
  enrollment_end date;
BEGIN
  SELECT start_date, end_date INTO enrollment_start, enrollment_end
  FROM class_enrollments
  WHERE id = NEW.enrollment_id;

  IF NOT FOUND THEN
    RAISE EXCEPTION 'enrollment % does not exist', NEW.enrollment_id;
  END IF;
  IF NEW.attendance_date < enrollment_start
     OR (enrollment_end IS NOT NULL AND NEW.attendance_date > enrollment_end) THEN
    RAISE EXCEPTION 'attendance date must be within enrollment period';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER attendance_within_enrollment
BEFORE INSERT OR UPDATE OF enrollment_id, attendance_date ON attendance_records
FOR EACH ROW EXECUTE FUNCTION check_attendance_within_enrollment();

CREATE FUNCTION enforce_attendance_system_close() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  actor_status varchar(20);
BEGIN
  IF NEW.source = 'system_close' THEN
    SELECT status INTO actor_status FROM users WHERE id = NEW.recorded_by_user_id;
    IF actor_status IS DISTINCT FROM 'system' THEN
      RAISE EXCEPTION 'system_close must be recorded by the system user';
    END IF;
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER attendance_system_close_user
BEFORE INSERT ON attendance_records
FOR EACH ROW EXECUTE FUNCTION enforce_attendance_system_close();

CREATE FUNCTION prevent_closed_attendance_insert() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  is_closed boolean;
BEGIN
  SELECT EXISTS (
    SELECT 1
    FROM attendance_day_closures cl
    JOIN class_enrollments e ON e.class_id = cl.class_id
    WHERE e.id = NEW.enrollment_id
      AND cl.attendance_date = NEW.attendance_date
  ) INTO is_closed;

  IF is_closed AND NEW.source NOT IN ('system_close','backfill') THEN
    RAISE EXCEPTION 'attendance day is closed; use a correction';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER attendance_closed_insert
BEFORE INSERT ON attendance_records
FOR EACH ROW EXECUTE FUNCTION prevent_closed_attendance_insert();

CREATE FUNCTION prevent_closed_attendance_update() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  is_closed boolean;
  has_correction boolean;
BEGIN
  IF NEW.status IS NOT DISTINCT FROM OLD.status
     AND NEW.check_in_at IS NOT DISTINCT FROM OLD.check_in_at
     AND NEW.notes IS NOT DISTINCT FROM OLD.notes THEN
    RETURN NEW;
  END IF;

  SELECT EXISTS (
    SELECT 1
    FROM attendance_day_closures cl
    JOIN class_enrollments e ON e.class_id = cl.class_id
    WHERE e.id = OLD.enrollment_id
      AND cl.attendance_date = OLD.attendance_date
  ) INTO is_closed;

  IF is_closed THEN
    SELECT EXISTS (
      SELECT 1 FROM attendance_corrections
      WHERE attendance_record_id = OLD.id
        AND corrected_at = now()
    ) INTO has_correction;
    IF NOT has_correction THEN
      RAISE EXCEPTION 'attendance day is closed; correction row required';
    END IF;
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER attendance_closed_update
BEFORE UPDATE OF status, check_in_at, notes ON attendance_records
FOR EACH ROW EXECUTE FUNCTION prevent_closed_attendance_update();

CREATE FUNCTION prevent_attendance_delete() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'attendance records are append-only';
END;
$$;

CREATE TRIGGER attendance_no_delete
BEFORE DELETE ON attendance_records
FOR EACH ROW EXECUTE FUNCTION prevent_attendance_delete();

CREATE FUNCTION prevent_correction_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'attendance corrections are append-only';
END;
$$;

CREATE TRIGGER attendance_corrections_append_only
BEFORE UPDATE OR DELETE ON attendance_corrections
FOR EACH ROW EXECUTE FUNCTION prevent_correction_mutation();

CREATE TRIGGER attendance_corrections_no_truncate
BEFORE TRUNCATE ON attendance_corrections
FOR EACH STATEMENT EXECUTE FUNCTION prevent_correction_mutation();

CREATE FUNCTION attendance_day_is_effective(p_class_id uuid, p_date date) RETURNS boolean
LANGUAGE sql STABLE AS $$
  SELECT EXISTS (
    SELECT 1
    FROM classes c
    JOIN academic_years ay ON ay.id = c.academic_year_id
    WHERE c.id = p_class_id
      AND p_date BETWEEN ay.start_date AND ay.end_date
      AND EXISTS (
        SELECT 1 FROM school_schedules ss
        WHERE ss.weekday = EXTRACT(ISODOW FROM p_date)::smallint
      )
      AND NOT EXISTS (
        SELECT 1 FROM school_holidays h
        WHERE h.holiday_date = p_date
      )
  );
$$;

INSERT INTO school_settings (id, school_name, timezone, attendance_auto_close_time)
VALUES (1, 'Sekolah', 'Asia/Jakarta', '18:00')
ON CONFLICT (id) DO NOTHING;
