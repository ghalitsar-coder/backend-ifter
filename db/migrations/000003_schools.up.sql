-- Tenant boundary: one registered school owns its users, academic years, and
-- therefore its classes and attendance sessions.
CREATE TABLE schools (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    npsn       VARCHAR(8)   NOT NULL,
    nama       VARCHAR(120) NOT NULL,
    jenjang    VARCHAR(20)  NOT NULL,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT uq_schools_npsn UNIQUE (npsn),
    CONSTRAINT chk_schools_npsn CHECK (npsn ~ '^[0-9]{8}$')
);

-- The single school the existing data belongs to.
INSERT INTO schools (id, npsn, nama, jenjang) OVERRIDING SYSTEM VALUE
VALUES (1, '00000000', 'SD Islam Bakti Asih', 'SD');
SELECT setval(pg_get_serial_sequence('schools', 'id'), (SELECT max(id) FROM schools));

ALTER TABLE users ADD COLUMN school_id BIGINT REFERENCES schools (id) ON DELETE RESTRICT;
UPDATE users SET school_id = 1;
ALTER TABLE users ALTER COLUMN school_id SET NOT NULL;
-- Registrations use the admin email as the login name.
ALTER TABLE users ALTER COLUMN username TYPE VARCHAR(254);
CREATE INDEX idx_users_school_id ON users (school_id);

ALTER TABLE academic_years ADD COLUMN school_id BIGINT REFERENCES schools (id) ON DELETE RESTRICT;
UPDATE academic_years SET school_id = 1;
ALTER TABLE academic_years ALTER COLUMN school_id SET NOT NULL;
CREATE INDEX idx_academic_years_school_id ON academic_years (school_id);

-- These keys were global before tenancy, so a second school could never reuse a
-- period name or hold its own active year. Scope them per school.
ALTER TABLE academic_years DROP CONSTRAINT uq_academic_years;
ALTER TABLE academic_years ADD CONSTRAINT uq_academic_years UNIQUE (school_id, nama, semester);
DROP INDEX uq_academic_years_active;
CREATE UNIQUE INDEX uq_academic_years_active ON academic_years (school_id) WHERE is_active;

-- Holidays hang off an academic year, so the same date can exist for each school.
ALTER TABLE holidays DROP CONSTRAINT uq_holidays_tanggal;
ALTER TABLE holidays ADD CONSTRAINT uq_holidays_academic_year_tanggal UNIQUE (academic_year_id, tanggal);
