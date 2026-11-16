DROP INDEX IF EXISTS idx_qr_sessions_school_tanggal;

ALTER TABLE qr_sessions
    DROP CONSTRAINT IF EXISTS uq_qr_sessions_school_tanggal_tipe,
    ADD CONSTRAINT uq_qr_sessions_tanggal_tipe UNIQUE (tanggal, tipe_sesi),
    DROP CONSTRAINT IF EXISTS fk_qr_sessions_creator_school,
    DROP COLUMN IF EXISTS school_id;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS uq_users_id_school;