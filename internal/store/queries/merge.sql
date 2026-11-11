-- name: FindCompanies :many
-- Companies matching a merge argument: its id, or its name (any case) (M6-06).
SELECT id, name, board_type, active FROM companies
WHERE id::text = @arg::text OR lower(name) = lower(@arg::text)
ORDER BY (board_type IS NULL), id;

-- name: MergeCounts :one
-- What a merge of @from_id would move.
SELECT
	(SELECT count(*) FROM postings WHERE company_id = @from_id::bigint) AS postings,
	(SELECT count(*) FROM jobs WHERE company_id = @from_id::bigint) AS jobs,
	(SELECT count(*) FROM applications WHERE company_id = @from_id::bigint) AS applications;

-- name: MergeConflicts :many
-- Title keys both companies have a current application for: they can't both move into one
-- (earlier, set-aside applications can: ADR-0032).
SELECT f.title_key FROM applications f
JOIN applications t ON t.company_id = @into_id::bigint AND t.title_key = f.title_key
	AND t.archived_at IS NULL
WHERE f.company_id = @from_id::bigint AND f.archived_at IS NULL ORDER BY f.title_key;

-- name: MovePostings :execrows
UPDATE postings SET company_id = @into_id::bigint WHERE company_id = @from_id::bigint;

-- name: MoveJobs :execrows
UPDATE jobs SET company_id = @into_id::bigint WHERE company_id = @from_id::bigint;

-- name: MoveApplications :execrows
UPDATE applications SET company_id = @into_id::bigint WHERE company_id = @from_id::bigint;
