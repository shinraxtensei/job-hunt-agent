-- name: AddSkip :one
-- A skip of the lead with posting @posting_id, after SetStatus made it skipped.
INSERT INTO skips (application_id, reason, note, score)
SELECT a.id, sqlc.narg('reason')::text, sqlc.narg('note')::text, @score::jsonb
FROM applications a JOIN jobs j ON j.company_id = a.company_id AND j.title_key = a.title_key
WHERE j.posting_id = @posting_id::bigint AND a.archived_at IS NULL
RETURNING id;

-- name: SetSkipReason :execrows
-- The reason given just after a skip: the lead's latest skip, while it is still skipped.
UPDATE skips SET reason = sqlc.narg('reason')::text, note = sqlc.narg('note')::text
WHERE id = (
	SELECT s.id FROM skips s
	JOIN applications a ON a.id = s.application_id
	JOIN jobs j ON j.company_id = a.company_id AND j.title_key = a.title_key
	WHERE j.posting_id = @posting_id::bigint AND a.status = 'skipped' AND a.archived_at IS NULL
	ORDER BY s.at DESC, s.id DESC LIMIT 1
);

-- name: ListSkips :many
-- Every skip, newest first, with its lead: M8's input.
SELECT s.id, a.posting_id, c.name AS company, j.title, s.reason, s.note, s.score, s.at
FROM skips s
JOIN applications a ON a.id = s.application_id
JOIN companies c ON c.id = a.company_id
JOIN jobs j ON j.posting_id = a.posting_id
ORDER BY s.at DESC, s.id DESC;
