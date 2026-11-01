-- name: UpsertPosting :one
-- Inserts a new posting. For one already stored, only last_checked_at moves: the
-- first-seen payload, URL and title are kept. inserted is true for a new row (xmax is
-- 0 only on a row this statement inserted rather than updated).
INSERT INTO postings (company_id, source, source_posting_id, url, title, raw_payload)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (source, source_posting_id) DO UPDATE SET last_checked_at = now()
RETURNING (xmax = 0)::boolean AS inserted;
