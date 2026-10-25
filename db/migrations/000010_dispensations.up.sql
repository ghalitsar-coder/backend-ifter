-- Beyond-MVP dispensations package (docs/API_ADDENDUM.md §5/§6.2, migration 000010).
-- Student permit/excuse (dispensation) requests with an optional private document.
-- class_id is a snapshot of the student's current enrollment taken at create time;
-- documents live in the private imports storage dir (never a static route), so the
-- row keeps only the original file name plus an opaque stored key.
CREATE TABLE dispensations (
  id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  student_id          uuid NOT NULL REFERENCES students(id),
  class_id            uuid REFERENCES classes(id),
  type                varchar(20) NOT NULL CHECK (type IN ('sick','late','permit','other')),
  reason              text NOT NULL,
  start_date          date NOT NULL,
  end_date            date,
  status              varchar(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected')),
  document_file_name  varchar(255),
  document_stored_key varchar(100),
  submitted_by        uuid NOT NULL REFERENCES users(id),
  verified_by         uuid REFERENCES users(id),
  verified_at         timestamptz,
  note                text,
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now(),
  -- One request per student, type and start day blocks duplicates (PRD §11).
  CONSTRAINT dispensations_student_type_day_uq UNIQUE (student_id, type, start_date),
  CONSTRAINT dispensations_end_after_start CHECK (end_date IS NULL OR end_date >= start_date),
  CONSTRAINT dispensations_reason_not_blank CHECK (btrim(reason) <> ''),
  CONSTRAINT dispensations_document_pair CHECK (
    (document_file_name IS NULL) = (document_stored_key IS NULL)
  ),
  CONSTRAINT dispensations_verifier_stamp_pair CHECK (
    (verified_by IS NULL) = (verified_at IS NULL)
  )
);

CREATE INDEX dispensations_student_date_idx ON dispensations (student_id, start_date);
CREATE INDEX dispensations_status_date_idx ON dispensations (status, start_date);
CREATE INDEX dispensations_class_date_idx ON dispensations (class_id, start_date);
