DROP TRIGGER IF EXISTS attendance_session_records_no_delete ON attendance_session_records;
DROP FUNCTION IF EXISTS prevent_session_record_delete();
DROP TRIGGER IF EXISTS attendance_session_records_open_update ON attendance_session_records;
DROP TRIGGER IF EXISTS attendance_session_records_open_insert ON attendance_session_records;
DROP FUNCTION IF EXISTS prevent_closed_session_record();
DROP TABLE IF EXISTS attendance_session_records;
DROP TABLE IF EXISTS attendance_sessions;
DROP TABLE IF EXISTS teaching_assignments;
DROP TABLE IF EXISTS subjects;
