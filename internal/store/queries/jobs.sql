-- name: UpsertJob :exec
-- Writes a posting's normalised record (ADR-0008), replacing any older one.
INSERT INTO jobs (posting_id, company_id, title, url, locations, country_codes, workplace,
	team, department, employment_type, published_at, normaliser_version, posting_hash, title_key,
	role_family, seniority, experience_min_years, remote_scopes, countries, classifier_version)
VALUES (@posting_id, @company_id, @title, @url, @locations, @country_codes, @workplace,
	@team, @department, @employment_type, @published_at, @normaliser_version, @posting_hash, @title_key,
	@role_family, @seniority, @experience_min_years, @remote_scopes, @countries, @classifier_version)
ON CONFLICT (posting_id) DO UPDATE SET
	company_id = EXCLUDED.company_id, title = EXCLUDED.title, url = EXCLUDED.url,
	locations = EXCLUDED.locations, country_codes = EXCLUDED.country_codes,
	workplace = EXCLUDED.workplace, team = EXCLUDED.team, department = EXCLUDED.department,
	employment_type = EXCLUDED.employment_type, published_at = EXCLUDED.published_at,
	normaliser_version = EXCLUDED.normaliser_version, posting_hash = EXCLUDED.posting_hash,
	title_key = EXCLUDED.title_key, role_family = EXCLUDED.role_family, seniority = EXCLUDED.seniority,
	experience_min_years = EXCLUDED.experience_min_years, remote_scopes = EXCLUDED.remote_scopes,
	countries = EXCLUDED.countries, classifier_version = EXCLUDED.classifier_version, normalised_at = now();

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

-- name: ListJobsToClassify :many
-- The next batch (by posting id, after after_id) of jobs whose facets were made by an
-- older classifier version (ADR-0011).
SELECT j.posting_id, p.source, p.raw_payload
FROM jobs j
JOIN postings p ON p.id = j.posting_id
WHERE j.posting_id > @after_id AND j.classifier_version < @version::integer
ORDER BY j.posting_id
LIMIT @batch_size;

-- name: UpdateJobFacets :exec
UPDATE jobs SET role_family = @role_family, seniority = @seniority,
	experience_min_years = @experience_min_years, remote_scopes = @remote_scopes,
	countries = @countries, classifier_version = @classifier_version
WHERE posting_id = @posting_id;
