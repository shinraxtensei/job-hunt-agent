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
