CREATE TABLE import_jobs (
  id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  entity_type           varchar(30) NOT NULL CHECK (entity_type IN ('students','teachers')),
  import_mode           varchar(20) NOT NULL CHECK (import_mode IN ('create_only','upsert')),
  original_filename     varchar(255) NOT NULL,
  file_sha256           char(64) NOT NULL,
  file_storage_key      text,
  status                varchar(20) NOT NULL
                        CHECK (status IN ('validating','validated','invalid','processing',
                                          'completed','failed','cancelled')),
  total_rows            integer NOT NULL DEFAULT 0,
  valid_rows            integer NOT NULL DEFAULT 0,
  duplicate_rows        integer NOT NULL DEFAULT 0,
  invalid_rows          integer NOT NULL DEFAULT 0,
  success_rows          integer NOT NULL DEFAULT 0,
  started_by_user_id    uuid NOT NULL REFERENCES users(id),
  confirmed_by_user_id  uuid REFERENCES users(id),
  created_at            timestamptz NOT NULL DEFAULT now(),
  validated_at          timestamptz,
  confirmed_at          timestamptz,
  completed_at          timestamptz
);
CREATE INDEX import_jobs_status_idx ON import_jobs (status, created_at);

CREATE TABLE import_row_errors (
  id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  import_job_id uuid NOT NULL REFERENCES import_jobs(id),
  row_number    integer NOT NULL,
  column_name   varchar(100),
  error_code    varchar(50) NOT NULL,
  message       text NOT NULL
);
CREATE INDEX import_row_errors_job_idx ON import_row_errors (import_job_id, row_number);
