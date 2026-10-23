-- name: CreateItem :one
INSERT INTO items (name)
VALUES ($1)
RETURNING *;

-- name: GetItem :one
SELECT * FROM items
WHERE id = $1;

-- name: ListItems :many
SELECT * FROM items
ORDER BY created_at DESC, id DESC
LIMIT $1 OFFSET $2;

-- name: CountItems :one
SELECT count(*) FROM items;
