DROP INDEX IF EXISTS attendance_correction_evidence_record_idx;
DROP TABLE IF EXISTS attendance_correction_evidence;
ALTER TABLE attendance_corrections DROP COLUMN IF EXISTS evidence_hash;
