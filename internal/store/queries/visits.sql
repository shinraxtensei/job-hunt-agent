-- name: LatestVisits :many
-- The current visit and the one before it, newest first.
SELECT id, started_at, last_seen_at, marked_at FROM visits ORDER BY started_at DESC, id DESC LIMIT 2;

-- name: StartVisit :one
INSERT INTO visits (started_at, last_seen_at) VALUES (sqlc.arg(at_time)::timestamptz, sqlc.arg(at_time)::timestamptz)
RETURNING id;

-- name: ExtendVisit :exec
UPDATE visits SET last_seen_at = sqlc.arg(at_time)::timestamptz WHERE id = sqlc.arg(id)::bigint;

-- name: MarkVisitSeen :execrows
UPDATE visits SET marked_at = sqlc.arg(at_time)::timestamptz, last_seen_at = sqlc.arg(at_time)::timestamptz
WHERE id = (SELECT v.id FROM visits v ORDER BY v.started_at DESC, v.id DESC LIMIT 1);

-- name: ClosedSince :many
-- Leads you acted on (any status but new or skipped) with a posting closed after since, by
-- group: the group's canonical posting changes when one closes (ADR-0009).
SELECT a.company_id, a.title_key, max(p.closed_at)::timestamptz AS closed_at
FROM postings p
JOIN jobs j ON j.posting_id = p.id
JOIN applications a ON a.company_id = j.company_id AND a.title_key = j.title_key
WHERE p.closed_at > sqlc.arg(since)::timestamptz AND a.status NOT IN ('new', 'skipped')
GROUP BY a.company_id, a.title_key;
