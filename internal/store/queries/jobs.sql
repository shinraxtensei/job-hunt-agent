-- name: UpsertJob :exec
-- Writes a posting's normalised record (ADR-0008), replacing any older one.
INSERT INTO jobs (posting_id, company_id, title, url, locations, country_codes, workplace,
	team, department, employment_type, published_at, normaliser_version, posting_hash)
VALUES (@posting_id, @company_id, @title, @url, @locations, @country_codes, @workplace,
	@team, @department, @employment_type, @published_at, @normaliser_version, @posting_hash)
ON CONFLICT (posting_id) DO UPDATE SET
	company_id = EXCLUDED.company_id, title = EXCLUDED.title, url = EXCLUDED.url,
	locations = EXCLUDED.locations, country_codes = EXCLUDED.country_codes,
	workplace = EXCLUDED.workplace, team = EXCLUDED.team, department = EXCLUDED.department,
	employment_type = EXCLUDED.employment_type, published_at = EXCLUDED.published_at,
	normaliser_version = EXCLUDED.normaliser_version, posting_hash = EXCLUDED.posting_hash,
	normalised_at = now();

-- name: ListPostingsToNormalise :many
-- The next batch (by posting id, after after_id) of postings whose job record is missing,
-- made by an older normaliser, or made from an older payload.
SELECT p.id, p.company_id, p.source, p.raw_payload, p.content_hash
FROM postings p
LEFT JOIN jobs j ON j.posting_id = p.id
WHERE p.id > @after_id
	AND (j.posting_id IS NULL OR j.normaliser_version < @version::integer OR j.posting_hash <> p.content_hash)
ORDER BY p.id
LIMIT @batch_size;
