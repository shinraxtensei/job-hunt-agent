-- name: ListCollectableCompanies :many
-- Companies a run fetches: on the watch list (active) and not excluded.
SELECT id, name, board_type, board_slug FROM companies
WHERE active AND NOT excluded
ORDER BY id;

-- name: StartRun :one
INSERT INTO collector_runs DEFAULT VALUES RETURNING id;

-- name: FinishRun :exec
UPDATE collector_runs
SET ended_at = now(), outcome = @outcome,
	companies_attempted = @companies_attempted, companies_failed = @companies_failed,
	companies_skipped = @companies_skipped,
	postings_new = @postings_new, postings_seen = @postings_seen, postings_changed = @postings_changed
WHERE id = @id;

-- name: AddRunError :exec
INSERT INTO collector_run_errors (run_id, company_id, error) VALUES ($1, $2, $3);
