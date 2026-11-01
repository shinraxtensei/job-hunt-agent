-- name: UpsertPosting :one
-- Stores one posting (ADR-0007). New: inserted with its hash. Already stored: the
-- payload, hash, URL and title are replaced and last_checked_at moves; last_changed_at
-- moves only when the hash differs, so an identical payload changes nothing else.
-- content_hash is computed here, from Postgres's canonical jsonb text, so key order and
-- whitespace in the source never count as a change. changed compares against the hash
-- read before this statement (the CTE sees the old row).
WITH old AS (
	SELECT content_hash FROM postings
	WHERE source = @source AND source_posting_id = @source_posting_id
)
INSERT INTO postings (company_id, source, source_posting_id, url, title, raw_payload, content_hash)
VALUES (@company_id, @source, @source_posting_id, @url, @title, @raw_payload,
	sha256(convert_to(CAST(sqlc.arg(raw_payload) AS jsonb)::text, 'UTF8')))
ON CONFLICT (source, source_posting_id) DO UPDATE SET
	last_checked_at = now(),
	raw_payload     = EXCLUDED.raw_payload,
	content_hash    = EXCLUDED.content_hash,
	url             = EXCLUDED.url,
	title           = EXCLUDED.title,
	last_changed_at = CASE WHEN postings.content_hash = EXCLUDED.content_hash
		THEN postings.last_changed_at ELSE now() END
RETURNING id, content_hash, (xmax = 0)::boolean AS inserted,
	(xmax <> 0 AND (SELECT content_hash FROM old) IS DISTINCT FROM content_hash)::boolean AS changed;
