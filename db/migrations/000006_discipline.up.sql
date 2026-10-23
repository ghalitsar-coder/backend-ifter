CREATE FUNCTION school_local_date(p_timestamp timestamptz) RETURNS date
LANGUAGE sql STABLE AS $$
  SELECT (p_timestamp AT TIME ZONE COALESCE(
    (SELECT timezone FROM school_settings WHERE id = 1), 'Asia/Jakarta'
  ))::date;
$$;

CREATE TABLE discipline_categories (
  id                         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code                       varchar(50) UNIQUE,
  name                       varchar(100) NOT NULL,
  description                text,
  default_points             integer CHECK (default_points >= 0),
  auto_on_attendance_status  varchar(20)
                             CHECK (auto_on_attendance_status IN ('late')),
  is_active                  boolean NOT NULL DEFAULT true,
  created_at                 timestamptz NOT NULL DEFAULT now(),
  updated_at                 timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX discipline_categories_name_uq
  ON discipline_categories (lower(name));
CREATE UNIQUE INDEX discipline_categories_auto_uq
  ON discipline_categories (auto_on_attendance_status)
  WHERE auto_on_attendance_status IS NOT NULL;

CREATE TABLE discipline_records (
  id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  student_id           uuid NOT NULL,
  enrollment_id        uuid NOT NULL,
  category_id          uuid NOT NULL REFERENCES discipline_categories(id),
  occurred_at          timestamptz NOT NULL,
  description          text NOT NULL,
  points               integer NOT NULL DEFAULT 0 CHECK (points >= 0),
  status               varchar(20) NOT NULL DEFAULT 'new'
                       CHECK (status IN ('new','under_review','in_followup','completed','cancelled')),
  attendance_record_id uuid REFERENCES attendance_records(id),
  recorded_by_user_id  uuid NOT NULL REFERENCES users(id),
  created_at           timestamptz NOT NULL DEFAULT now(),
  updated_at           timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (enrollment_id, student_id) REFERENCES class_enrollments (id, student_id)
);
CREATE INDEX discipline_student_idx  ON discipline_records (student_id, occurred_at);
CREATE INDEX discipline_category_idx ON discipline_records (category_id, occurred_at);
CREATE INDEX discipline_status_idx   ON discipline_records (status);
CREATE UNIQUE INDEX discipline_one_per_attendance
  ON discipline_records (attendance_record_id, category_id)
  WHERE attendance_record_id IS NOT NULL AND status <> 'cancelled';

CREATE TABLE discipline_followups (
  id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  discipline_record_id uuid NOT NULL REFERENCES discipline_records(id),
  status               varchar(20) NOT NULL
                       CHECK (status IN ('new','under_review','in_followup','completed','cancelled')),
  notes                text,
  updated_by_user_id   uuid NOT NULL REFERENCES users(id),
  created_at           timestamptz NOT NULL DEFAULT now(),
  CHECK (status <> 'cancelled' OR (notes IS NOT NULL AND btrim(notes) <> ''))
);
CREATE INDEX discipline_followups_record_idx
  ON discipline_followups (discipline_record_id, created_at);

CREATE FUNCTION check_discipline_within_enrollment() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  enrollment_start date;
  enrollment_end date;
  occurred_date date;
BEGIN
  SELECT start_date, end_date INTO enrollment_start, enrollment_end
  FROM class_enrollments
  WHERE id = NEW.enrollment_id;

  IF NOT FOUND THEN
    RAISE EXCEPTION 'enrollment % does not exist', NEW.enrollment_id;
  END IF;

  occurred_date := school_local_date(NEW.occurred_at);
  IF occurred_date < enrollment_start
     OR (enrollment_end IS NOT NULL AND occurred_date > enrollment_end) THEN
    RAISE EXCEPTION 'discipline date must be within enrollment period';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER discipline_within_enrollment
BEFORE INSERT OR UPDATE OF enrollment_id, occurred_at ON discipline_records
FOR EACH ROW EXECUTE FUNCTION check_discipline_within_enrollment();

CREATE FUNCTION prevent_discipline_followup_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'discipline follow-ups are append-only';
END;
$$;

CREATE TRIGGER discipline_followups_append_only
BEFORE UPDATE OR DELETE ON discipline_followups
FOR EACH ROW EXECUTE FUNCTION prevent_discipline_followup_mutation();

CREATE TRIGGER discipline_followups_no_truncate
BEFORE TRUNCATE ON discipline_followups
FOR EACH STATEMENT EXECUTE FUNCTION prevent_discipline_followup_mutation();

CREATE FUNCTION prevent_discipline_record_delete() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'discipline records are append-only';
END;
$$;

CREATE TRIGGER discipline_records_no_delete
BEFORE DELETE ON discipline_records
FOR EACH ROW EXECUTE FUNCTION prevent_discipline_record_delete();

CREATE TRIGGER discipline_records_no_truncate
BEFORE TRUNCATE ON discipline_records
FOR EACH STATEMENT EXECUTE FUNCTION prevent_discipline_record_delete();

CREATE FUNCTION sync_discipline_late() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  linked record;
BEGIN
  IF TG_OP = 'UPDATE' AND OLD.status = 'late' AND NEW.status <> 'late' THEN
    FOR linked IN
      SELECT dr.id, dr.status
      FROM discipline_records dr
      WHERE dr.attendance_record_id = NEW.id
        AND dr.status <> 'cancelled'
      FOR UPDATE
    LOOP
      UPDATE discipline_records
      SET status = 'cancelled', updated_at = now()
      WHERE id = linked.id;

      INSERT INTO discipline_followups (discipline_record_id, status, notes, updated_by_user_id)
      VALUES (linked.id, 'cancelled', 'dibatalkan otomatis: absensi dikoreksi', NEW.updated_by_user_id);

      INSERT INTO audit_logs (
        actor_user_id, action, entity_type, entity_id, before_data, after_data, reason
      ) VALUES (
        NEW.updated_by_user_id, 'discipline.cancel', 'discipline_record', linked.id,
        jsonb_build_object('status', linked.status),
        jsonb_build_object('status', 'cancelled'),
        'dibatalkan otomatis: absensi dikoreksi'
      );
    END LOOP;
  END IF;

  IF NEW.status = 'late' AND (TG_OP = 'INSERT' OR OLD.status IS DISTINCT FROM 'late') THEN
    INSERT INTO discipline_records (
      student_id, enrollment_id, category_id, occurred_at, description,
      points, status, attendance_record_id, recorded_by_user_id
    )
    SELECT NEW.student_id, NEW.enrollment_id, dc.id,
           COALESCE(
             NEW.check_in_at,
             (NEW.attendance_date::timestamp + time '12:00') AT TIME ZONE COALESCE(
               (SELECT timezone FROM school_settings WHERE id = 1), 'Asia/Jakarta'
             )
           ),
           'Terlambat berdasarkan absensi tanggal ' || NEW.attendance_date::text,
           COALESCE(dc.default_points, 0), 'new', NEW.id, NEW.recorded_by_user_id
    FROM discipline_categories dc
    WHERE dc.auto_on_attendance_status = 'late'
      AND dc.is_active
    ON CONFLICT DO NOTHING;
  END IF;

  RETURN NEW;
END;
$$;

CREATE TRIGGER attendance_sync_discipline_late
AFTER INSERT OR UPDATE OF status ON attendance_records
FOR EACH ROW EXECUTE FUNCTION sync_discipline_late();

INSERT INTO discipline_categories (code, name, description, default_points, auto_on_attendance_status)
VALUES ('late', 'Terlambat', 'Keterlambatan berdasarkan absensi', NULL, 'late')
ON CONFLICT (code) DO NOTHING;
