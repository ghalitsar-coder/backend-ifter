DROP TRIGGER IF EXISTS attendance_corrections_no_truncate ON attendance_corrections;
DROP TRIGGER IF EXISTS attendance_corrections_append_only ON attendance_corrections;
DROP TRIGGER IF EXISTS attendance_no_delete ON attendance_records;
DROP TRIGGER IF EXISTS attendance_closed_update ON attendance_records;
DROP TRIGGER IF EXISTS attendance_closed_insert ON attendance_records;
DROP TRIGGER IF EXISTS attendance_system_close_user ON attendance_records;
DROP TRIGGER IF EXISTS attendance_within_enrollment ON attendance_records;

DROP FUNCTION IF EXISTS prevent_correction_mutation();
DROP FUNCTION IF EXISTS prevent_attendance_delete();
DROP FUNCTION IF EXISTS prevent_closed_attendance_update();
DROP FUNCTION IF EXISTS prevent_closed_attendance_insert();
DROP FUNCTION IF EXISTS enforce_attendance_system_close();
DROP FUNCTION IF EXISTS check_attendance_within_enrollment();

DROP FUNCTION IF EXISTS attendance_day_is_effective(uuid, date);

DROP TABLE IF EXISTS attendance_corrections;
DROP TABLE IF EXISTS attendance_day_closures;
DROP TABLE IF EXISTS attendance_records;
