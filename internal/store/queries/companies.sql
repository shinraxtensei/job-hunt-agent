-- name: CreateCompany :one
INSERT INTO companies (name, board_type, board_slug)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetCompany :one
SELECT * FROM companies WHERE id = $1;

-- name: GetCompanyByBoard :one
SELECT * FROM companies WHERE board_type = $1 AND board_slug = $2;
