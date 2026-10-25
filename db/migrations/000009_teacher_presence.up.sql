-- Beyond-MVP teacher-presence package (docs/API_ADDENDUM.md §6.1, migration 000009).
-- School geofence configuration lives in its own single-row table (the same
-- single-row pattern as school_settings) so this package never rewrites the
-- attendance-owned settings row.
CREATE TABLE school_geofence (
  id            smallint PRIMARY KEY DEFAULT 1 CHECK (id = 1),
  latitude      double precision NOT NULL CHECK (latitude BETWEEN -90 AND 90),
  longitude     double precision NOT NULL CHECK (longitude BETWEEN -180 AND 180),
  radius_meters integer NOT NULL CHECK (radius_meters BETWEEN 1 AND 20000),
  is_enabled    boolean NOT NULL DEFAULT true,
  updated_by    uuid REFERENCES users(id),
  updated_at    timestamptz NOT NULL DEFAULT now()
);

-- Placeholder school coords (central Jakarta) until an administrator sets the
-- real location via PUT /api/v1/geofence. Seed style mirrors the
-- school_settings insert in 000005_attendance.
INSERT INTO school_geofence (id, latitude, longitude, radius_meters, is_enabled)
VALUES (1, -6.200000, 106.816666, 300, true)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE teacher_presences (
  id                        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  teacher_id                uuid NOT NULL REFERENCES teachers(id),
  presence_date             date NOT NULL,
  check_in_at               timestamptz NOT NULL,
  check_out_at              timestamptz,
  check_in_latitude         double precision NOT NULL CHECK (check_in_latitude BETWEEN -90 AND 90),
  check_in_longitude        double precision NOT NULL CHECK (check_in_longitude BETWEEN -180 AND 180),
  check_in_accuracy_meters  double precision NOT NULL CHECK (check_in_accuracy_meters >= 0),
  check_in_within_geofence  boolean NOT NULL,
  check_in_distance_meters  double precision NOT NULL CHECK (check_in_distance_meters >= 0),
  check_out_latitude        double precision CHECK (check_out_latitude BETWEEN -90 AND 90),
  check_out_longitude       double precision CHECK (check_out_longitude BETWEEN -180 AND 180),
  check_out_accuracy_meters double precision CHECK (check_out_accuracy_meters >= 0),
  check_out_within_geofence boolean,
  check_out_distance_meters double precision CHECK (check_out_distance_meters >= 0),
  notes                     text,
  created_at                timestamptz NOT NULL DEFAULT now(),
  updated_at                timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT teacher_presence_teacher_day_uq UNIQUE (teacher_id, presence_date),
  CONSTRAINT teacher_presence_checkout_after_checkin
    CHECK (check_out_at IS NULL OR check_out_at >= check_in_at),
  CONSTRAINT teacher_presence_check_out_complete
    CHECK ((check_out_at IS NULL) = (check_out_latitude IS NULL AND check_out_longitude IS NULL
                                     AND check_out_accuracy_meters IS NULL
                                     AND check_out_within_geofence IS NULL
                                     AND check_out_distance_meters IS NULL))
);

CREATE INDEX teacher_presences_date_idx ON teacher_presences (presence_date);
CREATE INDEX teacher_presences_teacher_date_idx ON teacher_presences (teacher_id, presence_date);
