CREATE INDEX students_status_idx ON students (status);
CREATE INDEX teachers_status_idx ON teachers (status);
CREATE INDEX discipline_occurred_idx ON discipline_records (occurred_at);
CREATE INDEX audit_logs_actor_created_idx ON audit_logs (actor_user_id, created_at);
CREATE INDEX enrollments_student_idx ON class_enrollments (student_id, start_date);
