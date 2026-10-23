-- name: ListTerms :many
SELECT id, academic_year_id, term_number, name, start_date, end_date
FROM terms
WHERE academic_year_id = sqlc.arg(academic_year_id)
ORDER BY term_number, id;

-- name: GetTerm :one
SELECT id, academic_year_id, term_number, name, start_date, end_date
FROM terms
WHERE id = sqlc.arg(id);

-- name: GetTermForUpdate :one
SELECT id, academic_year_id, term_number, name, start_date, end_date
FROM terms
WHERE id = sqlc.arg(id)
FOR UPDATE;

-- name: CreateTerm :one
INSERT INTO terms (academic_year_id, term_number, name, start_date, end_date)
VALUES (
  sqlc.arg(academic_year_id), sqlc.arg(term_number), sqlc.arg(name),
  sqlc.arg(start_date), sqlc.arg(end_date)
)
RETURNING id, academic_year_id, term_number, name, start_date, end_date;

-- name: UpdateTerm :one
UPDATE terms
SET term_number = sqlc.arg(term_number),
    name = sqlc.arg(name),
    start_date = sqlc.arg(start_date),
    end_date = sqlc.arg(end_date)
WHERE id = sqlc.arg(id)
RETURNING id, academic_year_id, term_number, name, start_date, end_date;
