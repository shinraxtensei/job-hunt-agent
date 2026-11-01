-- name: CreateCompany :one
INSERT INTO companies (name, board_type, board_slug)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetCompany :one
SELECT * FROM companies WHERE id = $1;
