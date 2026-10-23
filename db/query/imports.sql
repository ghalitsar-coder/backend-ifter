-- name: GetStudentByStudentNumber :one
SELECT id, student_number, national_student_number, full_name, gender, birth_date,
       admission_year, status, guardian_name, guardian_phone, created_at, updated_at
FROM students
WHERE student_number = sqlc.arg(student_number);

-- name: GetStudentByNationalStudentNumber :one
SELECT id, student_number, national_student_number, full_name, gender, birth_date,
       admission_year, status, guardian_name, guardian_phone, created_at, updated_at
FROM students
WHERE national_student_number = sqlc.arg(national_student_number);

-- name: GetTeacherByEmployeeNumber :one
SELECT id, user_id, employee_number, full_name, email, phone, status, created_at, updated_at
FROM teachers
WHERE employee_number = sqlc.arg(employee_number);

-- name: GetActiveAcademicYear :one
SELECT id, name, start_date, end_date, is_active, created_at, updated_at
FROM academic_years
WHERE is_active;

-- name: GetClassByYearAndName :one
SELECT c.id, c.name, c.status, ay.id AS academic_year_id,
       ay.name AS academic_year_name, ay.start_date AS academic_year_start,
       ay.end_date AS academic_year_end
FROM classes c
JOIN academic_years ay ON ay.id = c.academic_year_id
WHERE ay.name = sqlc.arg(academic_year_name)
  AND c.name = sqlc.arg(class_name);

-- name: GetOpenEnrollmentForStudent :one
SELECT id, student_id, class_id, start_date, end_date, created_by, created_at
FROM class_enrollments
WHERE student_id = sqlc.arg(student_id) AND end_date IS NULL
ORDER BY start_date DESC
LIMIT 1;

-- name: CreateImportJob :one
INSERT INTO import_jobs (
  entity_type, import_mode, original_filename, file_sha256, file_storage_key,
  status, total_rows, valid_rows, duplicate_rows, invalid_rows,
  started_by_user_id, validated_at
)
VALUES (
  sqlc.arg(entity_type), sqlc.arg(import_mode), sqlc.arg(original_filename),
  sqlc.arg(file_sha256), sqlc.arg(file_storage_key), sqlc.arg(status),
  sqlc.arg(total_rows), sqlc.arg(valid_rows), sqlc.arg(duplicate_rows),
  sqlc.arg(invalid_rows), sqlc.arg(started_by_user_id), now()
)
RETURNING id, entity_type, import_mode, original_filename, file_sha256,
          file_storage_key, status, total_rows, valid_rows, duplicate_rows,
          invalid_rows, success_rows, started_by_user_id, confirmed_by_user_id,
          created_at, validated_at, confirmed_at, completed_at;

-- name: CreateImportRowError :exec
INSERT INTO import_row_errors (import_job_id, row_number, column_name, error_code, message)
VALUES (sqlc.arg(import_job_id), sqlc.arg(row_number), sqlc.narg(column_name),
        sqlc.arg(error_code), sqlc.arg(message));

-- name: ListImportRowErrors :many
SELECT id, import_job_id, row_number, column_name, error_code, message
FROM import_row_errors
WHERE import_job_id = sqlc.arg(import_job_id)
ORDER BY row_number, id;

-- name: GetImportJob :one
SELECT id, entity_type, import_mode, original_filename, file_sha256,
       file_storage_key, status, total_rows, valid_rows, duplicate_rows,
       invalid_rows, success_rows, started_by_user_id, confirmed_by_user_id,
       created_at, validated_at, confirmed_at, completed_at
FROM import_jobs
WHERE id = sqlc.arg(id);

-- name: ListImportJobs :many
SELECT id, entity_type, import_mode, original_filename, file_sha256,
       file_storage_key, status, total_rows, valid_rows, duplicate_rows,
       invalid_rows, success_rows, started_by_user_id, confirmed_by_user_id,
       created_at, validated_at, confirmed_at, completed_at
FROM import_jobs
WHERE (sqlc.narg(entity_type)::text IS NULL OR entity_type = sqlc.narg(entity_type))
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
ORDER BY created_at DESC, id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountImportJobs :one
SELECT count(*)
FROM import_jobs
WHERE (sqlc.narg(entity_type)::text IS NULL OR entity_type = sqlc.narg(entity_type))
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status));

-- name: StartImportJob :one
UPDATE import_jobs
SET status = 'processing', confirmed_by_user_id = sqlc.arg(confirmed_by_user_id),
    confirmed_at = now()
WHERE id = sqlc.arg(id) AND status = 'validated'
RETURNING id, entity_type, import_mode, original_filename, file_sha256,
          file_storage_key, status, total_rows, valid_rows, duplicate_rows,
          invalid_rows, success_rows, started_by_user_id, confirmed_by_user_id,
          created_at, validated_at, confirmed_at, completed_at;

-- name: CompleteImportJob :exec
UPDATE import_jobs
SET status = 'completed', success_rows = sqlc.arg(success_rows), completed_at = now()
WHERE id = sqlc.arg(id);

-- name: FailImportJob :execrows
UPDATE import_jobs
SET status = sqlc.arg(status), success_rows = 0, completed_at = now()
WHERE id = sqlc.arg(id) AND status IN ('validated', 'processing');

-- name: ListStudentsForExport :many
SELECT s.id, s.student_number, s.national_student_number, s.full_name, s.gender,
       s.birth_date, s.admission_year, s.status, s.guardian_name, s.guardian_phone,
       COALESCE(cur.name, '') AS current_class_name,
       COALESCE(ay.name, '') AS current_academic_year_name,
       cur.start_date AS current_enrollment_start
FROM students s
LEFT JOIN LATERAL (
  SELECT c.id, c.name, c.academic_year_id, e.start_date
  FROM class_enrollments e
  JOIN classes c ON c.id = e.class_id
  WHERE e.student_id = s.id
    AND e.start_date <= CURRENT_DATE
    AND (e.end_date IS NULL OR e.end_date >= CURRENT_DATE)
  ORDER BY e.start_date DESC
  LIMIT 1
) cur ON true
LEFT JOIN academic_years ay ON ay.id = cur.academic_year_id
WHERE (sqlc.narg(status)::text IS NULL OR s.status = sqlc.narg(status))
  AND (sqlc.narg(class_id)::uuid IS NULL OR cur.id = sqlc.narg(class_id))
  AND (
    sqlc.narg(search)::text IS NULL
    OR s.full_name ILIKE '%' || sqlc.narg(search)::text || '%'
    OR s.student_number ILIKE '%' || sqlc.narg(search)::text || '%'
  )
ORDER BY s.full_name, s.id;

-- name: ListTeachersForExport :many
SELECT id, user_id, employee_number, full_name, email, phone, status, created_at, updated_at
FROM teachers
WHERE (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status))
  AND (
    sqlc.narg(search)::text IS NULL
    OR full_name ILIKE '%' || sqlc.narg(search)::text || '%'
    OR employee_number ILIKE '%' || sqlc.narg(search)::text || '%'
  )
ORDER BY full_name, id;

-- name: ListImportJobsForFileCleanup :many
SELECT id, file_storage_key
FROM import_jobs
WHERE status IN ('completed', 'invalid', 'failed', 'cancelled')
  AND file_storage_key IS NOT NULL
  AND COALESCE(completed_at, created_at) < sqlc.arg(cutoff)::timestamptz
ORDER BY COALESCE(completed_at, created_at)
LIMIT 100;

-- name: ClearImportJobFile :exec
UPDATE import_jobs
SET file_storage_key = NULL
WHERE id = sqlc.arg(id);

-- name: DeleteOldImportRowErrors :exec
DELETE FROM import_row_errors e
USING import_jobs j
WHERE e.import_job_id = j.id
  AND j.status IN ('completed', 'invalid', 'failed', 'cancelled')
  AND COALESCE(j.completed_at, j.created_at) < sqlc.arg(cutoff)::timestamptz;
