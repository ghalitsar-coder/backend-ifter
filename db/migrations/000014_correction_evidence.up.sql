-- correction-evidence (docs/API_ADDENDUM.md §5, §6.6 + package spec):
-- 1) server-signed integrity hash for attendance corrections, and
-- 2) private document attachments for those corrections.
ALTER TABLE attendance_corrections ADD COLUMN evidence_hash text;

CREATE TABLE attendance_correction_evidence (
  id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  attendance_record_id uuid NOT NULL REFERENCES attendance_records(id),
  correction_id        uuid REFERENCES attendance_corrections(id),
  file_name            varchar(255) NOT NULL CHECK (btrim(file_name) <> ''),
  content_type         varchar(100) NOT NULL,
  size_bytes           bigint NOT NULL CHECK (size_bytes > 0),
  stored_key           varchar(255) NOT NULL,
  uploaded_by          uuid NOT NULL REFERENCES users(id),
  note                 text,
  created_at           timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX attendance_correction_evidence_record_idx
  ON attendance_correction_evidence (attendance_record_id, created_at);
