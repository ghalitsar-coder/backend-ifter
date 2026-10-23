CREATE TABLE academic_years (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name       varchar(30) NOT NULL UNIQUE,
  start_date date NOT NULL,
  end_date   date NOT NULL,
  is_active  boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK (end_date > start_date),
  EXCLUDE USING gist (daterange(start_date, end_date, '[]') WITH &&)
);
CREATE UNIQUE INDEX academic_years_one_active ON academic_years (is_active) WHERE is_active;

CREATE TABLE terms (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  academic_year_id uuid NOT NULL REFERENCES academic_years(id),
  term_number      smallint NOT NULL CHECK (term_number BETWEEN 1 AND 4),
  name             varchar(30) NOT NULL,
  start_date       date NOT NULL,
  end_date         date NOT NULL,
  CHECK (end_date > start_date),
  UNIQUE (academic_year_id, term_number),
  EXCLUDE USING gist (academic_year_id WITH =, daterange(start_date, end_date, '[]') WITH &&)
);

CREATE TABLE classes (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  academic_year_id uuid NOT NULL REFERENCES academic_years(id),
  name             varchar(100) NOT NULL,
  grade_level      smallint NOT NULL CHECK (grade_level BETWEEN 1 AND 13),
  major            varchar(100),
  status           varchar(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active','archived')),
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now(),
  UNIQUE (academic_year_id, name)
);
CREATE INDEX classes_academic_year_idx ON classes (academic_year_id);

CREATE TABLE students (
  id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  student_number          varchar(50) NOT NULL,
  national_student_number varchar(20),
  full_name               varchar(150) NOT NULL,
  gender                  varchar(1) CHECK (gender IN ('L','P')),
  birth_date              date,
  admission_year          smallint NOT NULL CHECK (admission_year BETWEEN 1990 AND 2100),
  status                  varchar(20) NOT NULL DEFAULT 'active'
                          CHECK (status IN ('active','graduated','transferred','inactive')),
  guardian_name           varchar(150),
  guardian_phone          varchar(30),
  created_at              timestamptz NOT NULL DEFAULT now(),
  updated_at              timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT students_nis_uq UNIQUE (student_number),
  CONSTRAINT students_nisn_uq UNIQUE (national_student_number),
  CHECK (student_number = btrim(student_number) AND student_number <> '')
);

CREATE TABLE teachers (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id         uuid UNIQUE REFERENCES users(id),
  employee_number varchar(50) UNIQUE CHECK (employee_number <> ''),
  full_name       varchar(150) NOT NULL,
  email           varchar(255),
  phone           varchar(30),
  status          varchar(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active','inactive')),
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE class_enrollments (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  student_id uuid NOT NULL REFERENCES students(id),
  class_id   uuid NOT NULL REFERENCES classes(id),
  start_date date NOT NULL,
  end_date   date,
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK (end_date IS NULL OR end_date >= start_date),
  CONSTRAINT enrollments_id_student_uq UNIQUE (id, student_id),
  CONSTRAINT enrollments_no_overlap
    EXCLUDE USING gist (student_id WITH =, daterange(start_date, end_date, '[]') WITH &&)
);
CREATE INDEX enrollments_class_idx ON class_enrollments (class_id, start_date);

CREATE TABLE homeroom_assignments (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  teacher_id uuid NOT NULL REFERENCES teachers(id),
  class_id   uuid NOT NULL REFERENCES classes(id),
  start_date date NOT NULL,
  end_date   date,
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK (end_date IS NULL OR end_date >= start_date),
  CONSTRAINT homeroom_one_teacher_per_class
    EXCLUDE USING gist (class_id WITH =, daterange(start_date, end_date, '[]') WITH &&),
  CONSTRAINT homeroom_one_class_per_teacher
    EXCLUDE USING gist (teacher_id WITH =, daterange(start_date, end_date, '[]') WITH &&)
);
CREATE INDEX homeroom_teacher_idx ON homeroom_assignments (teacher_id);
CREATE INDEX homeroom_class_idx ON homeroom_assignments (class_id);

CREATE TABLE duty_assignments (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  teacher_id uuid NOT NULL REFERENCES teachers(id),
  weekday    smallint NOT NULL CHECK (weekday BETWEEN 1 AND 7),
  start_date date NOT NULL,
  end_date   date,
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK (end_date IS NULL OR end_date >= start_date),
  EXCLUDE USING gist (teacher_id WITH =, weekday WITH =, daterange(start_date, end_date, '[]') WITH &&)
);
CREATE INDEX duty_weekday_idx ON duty_assignments (weekday, start_date);

CREATE TABLE school_settings (
  id                         smallint PRIMARY KEY DEFAULT 1 CHECK (id = 1),
  school_name                varchar(150) NOT NULL,
  timezone                   varchar(50) NOT NULL DEFAULT 'Asia/Jakarta',
  attendance_auto_close_time time NOT NULL DEFAULT '18:00',
  updated_by                 uuid REFERENCES users(id),
  updated_at                 timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE school_schedules (
  weekday                smallint PRIMARY KEY CHECK (weekday BETWEEN 1 AND 7),
  start_time             time NOT NULL,
  late_tolerance_minutes smallint NOT NULL DEFAULT 0 CHECK (late_tolerance_minutes >= 0)
);

CREATE TABLE school_holidays (
  holiday_date date PRIMARY KEY,
  name         varchar(150) NOT NULL,
  created_by   uuid REFERENCES users(id),
  created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE FUNCTION check_term_within_academic_year() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  year_start date;
  year_end date;
BEGIN
  SELECT start_date, end_date INTO year_start, year_end
  FROM academic_years
  WHERE id = NEW.academic_year_id;

  IF NOT FOUND THEN
    RAISE EXCEPTION 'academic year % does not exist', NEW.academic_year_id;
  END IF;
  IF NEW.start_date < year_start OR NEW.end_date > year_end THEN
    RAISE EXCEPTION 'term dates must be within academic year %', NEW.academic_year_id;
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER terms_check_within_year
BEFORE INSERT OR UPDATE ON terms
FOR EACH ROW EXECUTE FUNCTION check_term_within_academic_year();

CREATE FUNCTION check_enrollment_within_class_year() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  year_start date;
  year_end date;
BEGIN
  SELECT ay.start_date, ay.end_date INTO year_start, year_end
  FROM classes c
  JOIN academic_years ay ON ay.id = c.academic_year_id
  WHERE c.id = NEW.class_id;

  IF NOT FOUND THEN
    RAISE EXCEPTION 'class % does not exist', NEW.class_id;
  END IF;
  IF NEW.start_date < year_start OR NEW.start_date > year_end THEN
    RAISE EXCEPTION 'enrollment start date must be within class academic year %', NEW.class_id;
  END IF;
  IF NEW.end_date IS NOT NULL AND NEW.end_date > year_end THEN
    RAISE EXCEPTION 'enrollment end date must be within class academic year %', NEW.class_id;
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER enrollments_check_within_class_year
BEFORE INSERT OR UPDATE ON class_enrollments
FOR EACH ROW EXECUTE FUNCTION check_enrollment_within_class_year();

-- O2 default: Monday-Friday, same start time and no tolerance.
-- The admin can change these rows without a schema change.
INSERT INTO school_schedules (weekday, start_time, late_tolerance_minutes) VALUES
  (1, '07:00', 0),
  (2, '07:00', 0),
  (3, '07:00', 0),
  (4, '07:00', 0),
  (5, '07:00', 0)
ON CONFLICT (weekday) DO NOTHING;
