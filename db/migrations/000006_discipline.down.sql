DROP TRIGGER IF EXISTS attendance_sync_discipline_late ON attendance_records;
DROP FUNCTION IF EXISTS sync_discipline_late();

DROP TRIGGER IF EXISTS discipline_records_no_truncate ON discipline_records;
DROP TRIGGER IF EXISTS discipline_records_no_delete ON discipline_records;
DROP FUNCTION IF EXISTS prevent_discipline_record_delete();

DROP TRIGGER IF EXISTS discipline_followups_no_truncate ON discipline_followups;
DROP TRIGGER IF EXISTS discipline_followups_append_only ON discipline_followups;
DROP FUNCTION IF EXISTS prevent_discipline_followup_mutation();

DROP TRIGGER IF EXISTS discipline_within_enrollment ON discipline_records;
DROP FUNCTION IF EXISTS check_discipline_within_enrollment();

DROP TABLE IF EXISTS discipline_followups;
DROP TABLE IF EXISTS discipline_records;
DROP TABLE IF EXISTS discipline_categories;

DROP FUNCTION IF EXISTS school_local_date(timestamptz);
