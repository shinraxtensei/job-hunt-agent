-- name: ListDocuments :many
-- A lead's exported documents, newest first, without their bytes (M5-04).
SELECT id, draft_id, kind, sha256, made_at, sent_at
FROM documents WHERE posting_id = @posting_id ORDER BY made_at DESC, id DESC;

-- name: MarkDocumentSent :one
-- The owner sent it (M5-04): sets sent_at once; marking it again returns the first time.
-- No row: no such document.
WITH marked AS (
	UPDATE documents AS u SET sent_at = now()
	WHERE u.id = @doc_id::bigint AND u.sent_at IS NULL RETURNING u.sent_at
)
SELECT m.sent_at FROM marked m
UNION ALL
SELECT d.sent_at FROM documents d
WHERE d.id = @doc_id::bigint AND NOT EXISTS (SELECT 1 FROM marked);
