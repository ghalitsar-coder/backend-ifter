-- device-registry (F10, docs/API_ADDENDUM.md §5): RFID readers, gate
-- controllers and tablets operated by the school. Devices are never deleted —
-- they are deactivated — so future tap data (F11) keeps a stable reference.
CREATE TABLE devices (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name         varchar(100) NOT NULL CHECK (btrim(name) <> ''),
  identifier   varchar(100) NOT NULL CHECK (btrim(identifier) <> ''),
  device_type  varchar(20) NOT NULL CHECK (device_type IN ('rfid_reader', 'gate', 'tablet', 'other')),
  location     varchar(200),
  notes        text,
  status       varchar(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
  last_seen_at timestamptz,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT devices_location_not_blank CHECK (location IS NULL OR btrim(location) <> ''),
  CONSTRAINT devices_notes_max CHECK (notes IS NULL OR char_length(notes) <= 2000)
);

-- The operator-facing serial is unique regardless of case ("RFID-GATE-01" and
-- "rfid-gate-01" are the same physical reader).
CREATE UNIQUE INDEX devices_identifier_key ON devices (lower(identifier));
CREATE INDEX devices_status_type_idx ON devices (status, device_type);
