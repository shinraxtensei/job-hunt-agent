-- name: ListCollectableCompanies :many
-- Companies a run fetches: on the watch list (active) and not excluded.
SELECT id, name, board_type::text AS board_type, board_slug::text AS board_slug FROM companies
WHERE active AND NOT excluded AND board_type IS NOT NULL
ORDER BY id;

-- name: StartRun :one
INSERT INTO collector_runs DEFAULT VALUES RETURNING id;

-- name: FinishRun :exec
UPDATE collector_runs
SET ended_at = now(), outcome = @outcome,
	companies_attempted = @companies_attempted, companies_failed = @companies_failed,
	companies_skipped = @companies_skipped,
	postings_new = @postings_new, postings_seen = @postings_seen, postings_changed = @postings_changed,
	jobs_failed = @jobs_failed, postings_closed = @postings_closed, postings_reopened = @postings_reopened,
	manual_rechecked = @manual_rechecked
WHERE id = @id;

-- name: AddRunError :exec
INSERT INTO collector_run_errors (run_id, company_id, error) VALUES ($1, $2, $3);
