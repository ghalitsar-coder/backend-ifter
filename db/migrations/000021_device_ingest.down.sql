DROP TABLE IF EXISTS device_taps;
DROP TABLE IF EXISTS student_cards;
DROP INDEX IF EXISTS devices_ingest_token_hash_key;
ALTER TABLE devices DROP COLUMN IF EXISTS ingest_token_hash;
