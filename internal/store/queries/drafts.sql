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
FROM drafts WHERE posting_id = @posting_id ORDER BY requested_at DESC, id DESC LIMIT 10;

-- name: DecideDraft :one
-- The owner's decision (M4-05): approve a draft that is ready for review, or discard one
-- that isn't decided yet. Nothing is sent either way (guardrail rule 6).
UPDATE drafts SET status = @status::text, decided_at = now()
WHERE id = @id::bigint AND (
	(@status::text = 'approved' AND status = 'ready_for_review') OR
	(@status::text = 'discarded' AND status IN ('ready_for_review', 'blocked', 'failed')))
RETURNING posting_id;

-- name: EvidenceLines :many
-- The text of evidence lines a draft cites, retired ones included (ADR-0024), so the page
-- shows what each sentence was built from.
SELECT id, text, retired FROM evidence_lines WHERE id = ANY(@ids::text[]);
