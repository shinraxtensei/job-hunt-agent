-- name: SetStatus :one
-- Sets a lead's status (M6-01): creates the group's application if needed and records the
-- change in its history. When @only_from is not empty, an existing application changes
-- only from those statuses (OI-11's automatic `drafted`). The first `applied` keeps the
-- posting as it is now. No row: no such lead, or the status didn't change.
WITH g AS (
	SELECT j.company_id, j.title_key FROM jobs j WHERE j.posting_id = @posting_id::bigint
), snap AS (
	SELECT jsonb_build_object('title', p.title, 'url', p.url, 'raw_payload', p.raw_payload,
		'content_hash', encode(p.content_hash, 'hex')) AS s
	FROM postings p WHERE p.id = @posting_id::bigint AND @status::text = 'applied'
), app AS (
	INSERT INTO applications (company_id, title_key, posting_id, status, applied_at, snapshot)
	SELECT g.company_id, g.title_key, @posting_id::bigint, @status::text,
		CASE WHEN @status::text = 'applied' THEN now() END, (SELECT s FROM snap)
	FROM g
	ON CONFLICT (company_id, title_key) DO UPDATE SET status = excluded.status, status_at = now(),
		applied_at = coalesce(applications.applied_at, excluded.applied_at),
		snapshot = coalesce(applications.snapshot, excluded.snapshot)
	WHERE applications.status <> excluded.status
		AND (cardinality(@only_from::text[]) = 0 OR applications.status = ANY (@only_from::text[]))
	RETURNING id
)
INSERT INTO application_events (application_id, status, auto)
SELECT app.id, @status::text, @auto::boolean FROM app
RETURNING application_id;

-- name: ListApplications :many
-- Every application, for the leads API and the board.
SELECT id, company_id, title_key, posting_id, status, status_at, applied_at
FROM applications;

-- name: GetApplication :one
-- The application of the lead with posting @posting_id (any member of its group).
SELECT a.id, a.status, a.status_at, a.applied_at, a.snapshot, a.created_at
FROM applications a
JOIN jobs j ON j.company_id = a.company_id AND j.title_key = a.title_key
WHERE j.posting_id = @posting_id::bigint;

-- name: ApplicationHistory :many
SELECT status, at, auto, coalesce(note, '')::text AS note FROM application_events
WHERE application_id = @application_id::bigint ORDER BY at DESC, id DESC;

-- name: SentDocuments :many
-- Documents marked sent for any posting of the application's group (M5-04), newest first.
SELECT d.id, d.kind, d.sent_at
FROM documents d
JOIN jobs j ON j.posting_id = d.posting_id
JOIN applications a ON a.company_id = j.company_id AND a.title_key = j.title_key
WHERE a.id = @application_id::bigint AND d.sent_at IS NOT NULL
ORDER BY d.sent_at DESC, d.id DESC;

-- name: LeadExists :one
SELECT EXISTS (SELECT 1 FROM jobs WHERE posting_id = @posting_id::bigint);
