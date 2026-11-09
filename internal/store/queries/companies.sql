-- name: CreateCompany :one
INSERT INTO companies (name, board_type, board_slug)
VALUES (@name, sqlc.arg(board_type)::text, sqlc.arg(board_slug)::text)
RETURNING *;

-- name: GetCompany :one
SELECT * FROM companies WHERE id = $1;

-- name: UpsertCompanyID :one
-- Returns the id of the company on this board, creating it if needed, in one statement
-- so concurrent runs can't create two rows. The no-op SET makes RETURNING work for an
-- existing row without changing it.
INSERT INTO companies (name, board_type, board_slug)
VALUES (@name, sqlc.arg(board_type)::text, sqlc.arg(board_slug)::text)
ON CONFLICT (board_type, board_slug) DO UPDATE SET board_slug = EXCLUDED.board_slug
RETURNING id;

-- name: UpsertWatchedCompany :one
-- Inserts a watch-list company, or updates it (and marks it active) if anything
-- differs. Returns no row when nothing changed; inserted is true for a new row.
INSERT INTO companies (name, board_type, board_slug, track, excluded, active)
VALUES (@name, sqlc.arg(board_type)::text, sqlc.arg(board_slug)::text, @track, @excluded, true)
ON CONFLICT (board_type, board_slug) DO UPDATE
	SET name = EXCLUDED.name, track = EXCLUDED.track, excluded = EXCLUDED.excluded, active = true
	WHERE (companies.name, companies.track, companies.excluded, companies.active)
		IS DISTINCT FROM (EXCLUDED.name, EXCLUDED.track, EXCLUDED.excluded, true)
RETURNING (xmax = 0)::boolean AS inserted;

-- name: DeactivateCompaniesNotIn :execrows
-- Marks every active company whose board ("board_type/board_slug") is not in
-- board_keys inactive. Slugs never contain "/" (the watch list rejects them).
UPDATE companies SET active = false
WHERE active AND board_type || '/' || board_slug <> ALL (@board_keys::text[]);

-- name: ListCompanies :many
SELECT id, name, coalesce(track, '')::text AS track, coalesce(board_type, '')::text AS board_type,
	coalesce(board_slug, '')::text AS board_slug, active, excluded
FROM companies
ORDER BY name, id;

-- name: EnsureManualBoardCompany :one
-- The company of a board posting added by link (ADR-0015): found by its board, or created
-- inactive so it isn't collected daily unless it joins the watch list.
INSERT INTO companies (name, board_type, board_slug, active)
VALUES (@name, sqlc.arg(board_type)::text, sqlc.arg(board_slug)::text, false)
ON CONFLICT (board_type, board_slug) DO UPDATE SET board_slug = EXCLUDED.board_slug
RETURNING id;

-- name: EnsureBoardlessCompany :one
-- A company typed with --company (no board), one row per name whatever the case.
INSERT INTO companies (name, active)
VALUES (@name, false)
ON CONFLICT (lower(name)) WHERE board_type IS NULL DO UPDATE SET name = companies.name
RETURNING id;

-- name: FindCompanyByName :one
-- A board posting's company (ADR-0029): by name, whatever the case, a watch-list row
-- (one with a board) first, so a company's own feed and the board group together. Never
-- the board's own row.
SELECT id FROM companies
WHERE lower(name) = lower(@name) AND board_type IS DISTINCT FROM sqlc.arg(board_type)::text
ORDER BY (board_type IS NULL), active DESC, id
LIMIT 1;

-- name: SetFetchedAt :exec
UPDATE companies SET fetched_at = now() WHERE id = @id;
