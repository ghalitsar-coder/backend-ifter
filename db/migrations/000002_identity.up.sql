CREATE TABLE users (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  username      varchar(100) NOT NULL,
  email         varchar(255),
  password_hash text,
  full_name     varchar(150) NOT NULL,
  status        varchar(20) NOT NULL DEFAULT 'pending'
                CHECK (status IN ('pending','active','inactive','locked','system')),
  last_login_at timestamptz,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  CHECK (username = btrim(username) AND username <> ''),
  CHECK (status IN ('pending','system') OR password_hash IS NOT NULL)
);
CREATE UNIQUE INDEX users_username_uq ON users (lower(username));
CREATE UNIQUE INDEX users_email_uq ON users (lower(email)) WHERE email IS NOT NULL;

CREATE TABLE user_credential_tokens (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    uuid NOT NULL REFERENCES users(id),
  purpose    varchar(20) NOT NULL CHECK (purpose IN ('invite','reset')),
  token_hash char(64) NOT NULL UNIQUE,
  expires_at timestamptz NOT NULL,
  used_at    timestamptz,
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE refresh_tokens (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     uuid NOT NULL REFERENCES users(id),
  token_hash  char(64) NOT NULL UNIQUE,
  expires_at  timestamptz NOT NULL,
  revoked_at  timestamptz,
  replaced_by uuid REFERENCES refresh_tokens(id) ON DELETE SET NULL,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX refresh_tokens_user_idx ON refresh_tokens (user_id, expires_at);

CREATE TABLE roles (
  code        varchar(50) PRIMARY KEY,
  name        varchar(100) NOT NULL,
  description text
);

CREATE TABLE user_roles (
  user_id     uuid NOT NULL REFERENCES users(id),
  role_code   varchar(50) NOT NULL REFERENCES roles(code),
  assigned_at timestamptz NOT NULL DEFAULT now(),
  assigned_by uuid REFERENCES users(id),
  PRIMARY KEY (user_id, role_code)
);

CREATE TABLE audit_logs (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  actor_user_id uuid REFERENCES users(id),
  action        varchar(100) NOT NULL,
  entity_type   varchar(100) NOT NULL,
  entity_id     uuid,
  request_id    varchar(100),
  before_data   jsonb,
  after_data    jsonb,
  reason        text,
  created_at    timestamptz NOT NULL DEFAULT now(),
  CHECK (action NOT IN ('discipline.cancel','discipline.points_change','attendance.backfill')
         OR (reason IS NOT NULL AND btrim(reason) <> ''))
);
CREATE INDEX audit_logs_created_idx ON audit_logs (created_at);
CREATE INDEX audit_logs_entity_idx ON audit_logs (entity_type, entity_id, created_at);

CREATE FUNCTION prevent_audit_log_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'audit_logs is append-only';
END;
$$;

CREATE TRIGGER audit_logs_append_only
BEFORE UPDATE OR DELETE ON audit_logs
FOR EACH ROW EXECUTE FUNCTION prevent_audit_log_mutation();

CREATE TRIGGER audit_logs_no_truncate
BEFORE TRUNCATE ON audit_logs
FOR EACH STATEMENT EXECUTE FUNCTION prevent_audit_log_mutation();

INSERT INTO roles (code, name, description) VALUES
  ('admin', 'Administrator Sekolah', 'Mengelola seluruh data sekolah dan pengguna.'),
  ('teacher', 'Guru / Wali Kelas', 'Mencatat absensi dan kedisiplinan sesuai cakupan kelas.'),
  ('duty_teacher', 'Guru Piket', 'Mencatat absensi pada hari piket yang aktif.'),
  ('principal', 'Kepala Sekolah', 'Melihat dashboard dan laporan; hanya baca.')
ON CONFLICT (code) DO NOTHING;

INSERT INTO users (username, full_name, status)
VALUES ('system', 'System', 'system')
ON CONFLICT DO NOTHING;
