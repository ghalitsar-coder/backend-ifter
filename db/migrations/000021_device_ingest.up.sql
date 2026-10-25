-- RFID tap ingest (F11): readers authenticate with a per-device token, send
-- tap batches, and the tap links to a student once an admin maps the card UID.
-- Card mappings live in their own table so the student read surfaces are
-- untouched. Devices stay referenced by device_taps, so retirement remains a
-- soft path.

-- SHA-256 hex of the bearer token handed to the reader; the token itself is
-- only ever returned once (create and rotate), never on reads.
ALTER TABLE devices ADD COLUMN ingest_token_hash text;
CREATE UNIQUE INDEX devices_ingest_token_hash_key ON devices (ingest_token_hash)
  WHERE ingest_token_hash IS NOT NULL;

CREATE TABLE student_cards (
  student_id uuid PRIMARY KEY REFERENCES students(id) ON DELETE CASCADE,
  card_uid   varchar(64) NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT student_cards_card_uid_check
    CHECK (btrim(card_uid) <> '' AND card_uid = btrim(card_uid))
);
CREATE UNIQUE INDEX student_cards_card_uid_key ON student_cards (card_uid);

CREATE TABLE device_taps (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  device_id   uuid NOT NULL REFERENCES devices(id),
  student_id  uuid REFERENCES students(id) ON DELETE SET NULL,
  card_uid    varchar(64) NOT NULL,
  occurred_at timestamptz NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  CHECK (btrim(card_uid) <> ''),
  CONSTRAINT device_taps_dedupe_uq UNIQUE (device_id, card_uid, occurred_at)
);
CREATE INDEX device_taps_student_idx ON device_taps (student_id, occurred_at DESC);
CREATE INDEX device_taps_occurred_idx ON device_taps (occurred_at DESC);
