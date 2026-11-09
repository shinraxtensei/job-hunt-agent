-- name: UpsertPosting :one
-- Stores one posting (ADR-0007). New: inserted with its hash. Already stored: the
-- payload, hash, URL and title are replaced and last_checked_at moves; last_changed_at
-- moves only when the hash differs, so an identical payload changes nothing else.
-- content_hash is computed here, from Postgres's canonical jsonb text, so key order and
-- whitespace in the source never count as a change. changed compares against the hash
-- read before this statement (the CTE sees the old row).
WITH old AS (
	SELECT content_hash, closed_at FROM postings
	WHERE source = @source AND source_posting_id = @source_posting_id
)
INSERT INTO postings (company_id, source, source_posting_id, url, title, raw_payload, content_hash)
VALUES (@company_id, @source, @source_posting_id, @url, @title, @raw_payload,
	sha256(convert_to(CAST(sqlc.arg(raw_payload) AS jsonb)::text, 'UTF8')))
ON CONFLICT (source, source_posting_id) DO UPDATE SET
	last_checked_at = now(),
	closed_at       = NULL, -- seen again: open (ADR-0010)
	raw_payload     = EXCLUDED.raw_payload,
	content_hash    = EXCLUDED.content_hash,
	url             = EXCLUDED.url,
	title           = EXCLUDED.title,
	last_changed_at = CASE WHEN postings.content_hash = EXCLUDED.content_hash
		THEN postings.last_changed_at ELSE now() END
RETURNING id, content_hash, (xmax = 0)::boolean AS inserted,
	(xmax <> 0 AND (SELECT content_hash FROM old) IS DISTINCT FROM content_hash)::boolean AS changed,
	(xmax <> 0 AND (SELECT closed_at FROM old) IS NOT NULL)::boolean AS reopened;

-- name: CountOpenPostings :one
SELECT count(*) FROM postings
WHERE company_id = @company_id AND source = @source AND closed_at IS NULL;

-- name: CountPostingsNotIn :one
-- How many of the company's open postings from this source ClosePostingsNotIn would close.
SELECT count(*) FROM postings
WHERE company_id = @company_id AND source = @source AND closed_at IS NULL
	AND source_posting_id <> ALL (@seen_ids::text[]);

-- name: ClosePostingsNotIn :execrows
-- Closes the company's open postings from this source that the fetched board no longer
-- lists (ADR-0010). Run only after a successful fetch, in the company's transaction.
UPDATE postings SET closed_at = now()
WHERE company_id = @company_id AND source = @source AND closed_at IS NULL
	AND source_posting_id <> ALL (@seen_ids::text[]);

-- name: GetPostingID :one
-- The posting stored under this source and id, if any (the duplicate check, ADR-0015).
SELECT id FROM postings WHERE source = @source AND source_posting_id = @source_posting_id;

-- name: MarkManual :exec
UPDATE postings SET origin = 'manual' WHERE id = @id;

-- name: SetRechecked :exec
UPDATE postings SET last_rechecked_at = now() WHERE id = @id;

-- name: CloseRechecked :execrows
-- A manual board posting its board no longer has (404): closed, and rechecked now.
UPDATE postings SET closed_at = coalesce(closed_at, now()), last_rechecked_at = now()
WHERE id = @id AND closed_at IS NULL;

-- name: ListManualToRecheck :many
-- Manual board postings of companies that aren't collected, not rechecked for 2 days.
SELECT p.id, p.source, p.source_posting_id, p.company_id, c.board_slug::text AS board_slug
FROM postings p
JOIN companies c ON c.id = p.company_id
WHERE p.origin = 'manual' AND p.source IN ('lever', 'greenhouse', 'ashby')
	AND NOT (c.active AND NOT c.excluded)
	AND c.board_slug IS NOT NULL
	AND (p.last_rechecked_at IS NULL OR p.last_rechecked_at < now() - interval '2 days')
ORDER BY p.id;

-- name: MarkBoard :exec
-- A posting from a board-wide source (ADR-0029). A manual add of the same posting stays manual.
UPDATE postings SET origin = 'board' WHERE id = @id AND origin = 'collected';

-- name: CountOpenSourcePostings :one
-- A board-wide source's open postings, across companies (ADR-0029).
SELECT count(*) FROM postings WHERE source = @source AND closed_at IS NULL;

-- name: CountSourcePostingsNotIn :one
SELECT count(*) FROM postings
WHERE source = @source AND closed_at IS NULL AND source_posting_id <> ALL (@seen_ids::text[]);

-- name: CloseSourcePostingsNotIn :execrows
-- ClosePostingsNotIn for a board-wide source: what its fetch no longer lists, any company.
UPDATE postings SET closed_at = now()
WHERE source = @source AND closed_at IS NULL AND source_posting_id <> ALL (@seen_ids::text[]);
