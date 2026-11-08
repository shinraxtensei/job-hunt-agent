-- name: ListFollowups :many
-- Applications applied on or before the due cutoff (OI-15: 7 days ago), still `applied` (a
-- later status ends the reminder), not done, not snoozed past now: with the lead, the
-- first contact and the owner's name.
SELECT a.id, a.posting_id, j.title, c.name AS company, a.applied_at,
	coalesce((SELECT k.name FROM contacts k WHERE k.application_id = a.id ORDER BY k.created_at, k.id LIMIT 1), '')::text AS contact,
	coalesce((SELECT e.facts ->> 'name' FROM evidence_items e WHERE e.id = 'person'), '')::text AS owner
FROM applications a
JOIN jobs j ON j.posting_id = a.posting_id
JOIN companies c ON c.id = a.company_id
WHERE a.status = 'applied' AND a.followup_done_at IS NULL
	AND a.applied_at <= sqlc.arg(due_before)::timestamptz
	AND (a.followup_snoozed_until IS NULL OR a.followup_snoozed_until <= sqlc.arg(at_time)::timestamptz)
ORDER BY a.applied_at, a.id;

-- name: FollowupAction :execrows
-- Done or snoozed (OI-15), kept on the application and as a history entry.
WITH a AS (
	UPDATE applications SET
		followup_done_at = CASE WHEN sqlc.arg(done)::boolean THEN now() ELSE followup_done_at END,
		followup_snoozed_until = sqlc.narg(snooze_until)::timestamptz
	WHERE id = sqlc.arg(id)::bigint AND status = 'applied'
	RETURNING id, status
)
INSERT INTO application_events (application_id, status, at, note)
SELECT a.id, a.status, now(), sqlc.arg(note)::text FROM a; -- the database's clock, like every history entry
