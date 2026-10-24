-- Destructive after registrations exist: dropping school_id removes the tenant
-- link, narrowing username can fail on registered email addresses, and restoring
-- the global active-year index fails while two schools are active.
ALTER TABLE holidays DROP CONSTRAINT IF EXISTS uq_holidays_academic_year_tanggal;
ALTER TABLE holidays ADD CONSTRAINT uq_holidays_tanggal UNIQUE (tanggal);

DROP INDEX IF EXISTS uq_academic_years_active;
CREATE UNIQUE INDEX uq_academic_years_active ON academic_years (is_active) WHERE is_active;
ALTER TABLE academic_years DROP CONSTRAINT IF EXISTS uq_academic_years;
ALTER TABLE academic_years ADD CONSTRAINT uq_academic_years UNIQUE (nama, semester);

DROP INDEX IF EXISTS idx_academic_years_school_id;
ALTER TABLE academic_years DROP COLUMN IF EXISTS school_id;

DROP INDEX IF EXISTS idx_users_school_id;
ALTER TABLE users DROP COLUMN IF EXISTS school_id;
ALTER TABLE users ALTER COLUMN username TYPE VARCHAR(50);

DROP TABLE IF EXISTS schools;
