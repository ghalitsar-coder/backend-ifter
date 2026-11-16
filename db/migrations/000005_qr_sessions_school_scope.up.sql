ALTER TABLE users
    ADD CONSTRAINT uq_users_id_school UNIQUE (id, school_id);

ALTER TABLE qr_sessions
    ADD COLUMN school_id BIGINT REFERENCES schools (id) ON DELETE RESTRICT;

UPDATE qr_sessions qr
SET school_id = u.school_id
FROM users u
WHERE u.id = qr.created_by;

ALTER TABLE qr_sessions
    ALTER COLUMN school_id SET NOT NULL,
    ADD CONSTRAINT fk_qr_sessions_creator_school
        FOREIGN KEY (created_by, school_id) REFERENCES users (id, school_id);

ALTER TABLE qr_sessions
    DROP CONSTRAINT uq_qr_sessions_tanggal_tipe,
    ADD CONSTRAINT uq_qr_sessions_school_tanggal_tipe UNIQUE (school_id, tanggal, tipe_sesi);

CREATE INDEX idx_qr_sessions_school_tanggal ON qr_sessions (school_id, tanggal);