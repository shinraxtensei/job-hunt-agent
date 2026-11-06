-- name: EnqueueExtract :execrows
-- The reconcile step's write (ADR-0001): one extract item per posting, re-queued only
-- when the posting's content changed. Idempotent and safe to run concurrently.
INSERT INTO work_items (posting_id, kind, content_hash)
SELECT unnest(@posting_ids::bigint[]), 'extract', unnest(@content_hashes::bytea[])
ON CONFLICT (posting_id, kind) DO UPDATE
SET content_hash = excluded.content_hash, state = 'pending', attempts = 0, run_after = now(),
	locked_until = NULL, claim_token = NULL, last_error = NULL, updated_at = now()
WHERE work_items.content_hash <> excluded.content_hash;

-- name: CountWorkItems :many
-- Items by state, for `collector queue` and GET /api/queue.
SELECT state, count(*)::bigint AS n FROM work_items GROUP BY state;

-- name: CurrentPromptVersion :one
-- The prompt version the worker runs for an agent (it records it at start-up).
SELECT version FROM agent_prompts WHERE agent = @agent;

-- name: OutdatedExtractions :many
-- Of these postings, the ones whose current content was extracted only with a prompt
-- older than @version (M3-04 AC7). Never-extracted postings are the queue's job.
SELECT e.posting_id FROM extractions e
JOIN postings p ON p.id = e.posting_id AND p.content_hash = e.content_hash
WHERE e.posting_id = ANY(@posting_ids::bigint[])
GROUP BY e.posting_id
HAVING max(e.prompt_version) < @version::int;

-- name: RequeueExtract :execrows
-- Puts these postings' extract items back to pending (not one a worker holds).
UPDATE work_items
SET state = 'pending', attempts = 0, run_after = now(), last_error = NULL, updated_at = now()
WHERE kind = 'extract' AND posting_id = ANY(@posting_ids::bigint[]) AND state <> 'running';

-- name: AverageExtractCost :one
-- The mean cost of a successful extraction so far, for estimates (ADR-0018's table is
-- per token; this is per posting, measured).
SELECT coalesce(avg(cost_usd), 0)::float8 AS usd, count(*)::bigint AS n
FROM model_calls WHERE agent = 'extract' AND outcome = 'ok';

-- name: RetryFailed :execrows
-- M3-04 AC8: failed items of a kind back to pending, with 0 attempts and no error.
UPDATE work_items
SET state = 'pending', attempts = 0, run_after = now(), last_error = NULL, updated_at = now()
WHERE state = 'failed' AND kind = @kind;

-- name: SetMatchedLocations :exec
-- The reconcile step's second write (ADR-0023): the default preset's matched location of
-- each shown group's canonical posting, for the scorer. Unchanged rows aren't touched.
UPDATE jobs j SET matched_location = x.loc
FROM (SELECT unnest(@posting_ids::bigint[]) AS id, unnest(@locations::text[]) AS loc) x
WHERE j.posting_id = x.id AND j.matched_location <> x.loc;
