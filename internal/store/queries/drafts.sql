-- name: RequestDraft :one
-- A draft request (M4-02, ADR-0025): one `requested` row for a scored lead, unless one is
-- already waiting. No row: nothing to request.
INSERT INTO drafts (posting_id)
SELECT @posting_id::bigint
WHERE EXISTS (SELECT 1 FROM scores WHERE posting_id = @posting_id::bigint)
  AND NOT EXISTS (SELECT 1 FROM drafts WHERE posting_id = @posting_id::bigint AND status = 'requested')
RETURNING id;

-- name: ListDrafts :many
-- A lead's drafts, newest first (M4-05 shows them). CV content: never logged.
SELECT id, status, variant, content, coverage, findings, evidence_version, prompt_version,
	model, error, requested_at, made_at, decided_at
FROM drafts WHERE posting_id = @posting_id ORDER BY id DESC LIMIT 10;
