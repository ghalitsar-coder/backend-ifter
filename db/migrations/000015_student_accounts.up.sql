-- student-accounts (docs/API_ADDENDUM.md §5, §6.7 + package spec, migration 000015):
-- the login link from a student row to its user account, mirroring the
-- existing teachers.user_id pattern (000003_master_data). It is what makes
-- the student's own /my/* surface and GET /students/:id/account resolvable
-- server-side from the authenticated subject (never from a client id).
ALTER TABLE students ADD COLUMN user_id uuid UNIQUE REFERENCES users(id);

-- One login can only ever own one student row: the UNIQUE above already
-- guarantees it; this partial index keeps the lookup cheap for /my/*.
CREATE INDEX students_user_id_lookup_idx ON students (user_id) WHERE user_id IS NOT NULL;
