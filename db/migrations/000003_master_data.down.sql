DROP TABLE IF EXISTS school_holidays;
DROP TABLE IF EXISTS school_schedules;
DROP TABLE IF EXISTS school_settings;
DROP TABLE IF EXISTS duty_assignments;
DROP TABLE IF EXISTS homeroom_assignments;
DROP TABLE IF EXISTS class_enrollments;
DROP TABLE IF EXISTS teachers;
DROP TABLE IF EXISTS students;
DROP TABLE IF EXISTS classes;
DROP TABLE IF EXISTS terms;
DROP TABLE IF EXISTS academic_years;

DROP FUNCTION IF EXISTS check_enrollment_within_class_year();
DROP FUNCTION IF EXISTS check_term_within_academic_year();
