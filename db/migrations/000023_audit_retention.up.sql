-- Allow the maintenance job to delete expired audit rows without weakening
-- the append-only guarantee: normal sessions (and any UPDATE/TRUNCATE) still
-- raise, and DELETE is only accepted inside a transaction that explicitly sets
-- the local `ifter.audit_retention` flag.
CREATE OR REPLACE FUNCTION prevent_audit_log_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'DELETE' AND current_setting('ifter.audit_retention', true) = 'on' THEN
    RETURN OLD;
  END IF;
  RAISE EXCEPTION 'audit_logs is append-only';
END;
$$;
