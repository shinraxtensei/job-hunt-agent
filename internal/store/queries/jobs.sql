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

-- name: ListGroupsForPresets :many
-- Every duplicate group of an active company with its canonical job's facets (ADR-0011),
-- and its canonical posting's id and content hash (the reconcile step, ADR-0001).
SELECT g.company_id, g.title_key, g.closed, coalesce(c.track, '')::text AS track, c.name AS company,
	j.title, j.role_family, j.seniority, j.experience_min_years, cp.id AS posting_id, cp.content_hash
FROM job_groups g
JOIN companies c ON c.id = g.company_id
JOIN jobs j ON j.posting_id = g.canonical_posting_id
JOIN postings cp ON cp.id = g.canonical_posting_id
WHERE c.active OR EXISTS (
	SELECT 1 FROM jobs m JOIN postings mp ON mp.id = m.posting_id
	WHERE m.company_id = g.company_id AND m.title_key = g.title_key
		AND (mp.origin = 'manual' OR (mp.origin = 'board' AND NOT c.excluded))
)
ORDER BY g.company_id, g.title_key;

-- name: ListJobPlaces :many
-- Each job's location facets, for evaluating its group's location rules; posting_id says
-- which member passed (ADR-0030).
SELECT posting_id, company_id, title_key, workplace, locations, remote_scopes, countries
FROM jobs
WHERE title_key <> '';

-- name: ListLeads :many
-- Every duplicate group of an active company, or with a manually added member
-- (ADR-0015) or a board member (ADR-0029), with its canonical job, for the leads API (ADR-0012). first_seen_at is the
-- group's: its earliest member's.
SELECT g.company_id, g.title_key, g.members, g.closed, g.locations::text[] AS locations,
	g.workplaces::text[] AS workplaces, c.name AS company, coalesce(c.track, '')::text AS track,
	j.posting_id, j.title, j.team, j.url, j.published_at, j.role_family, j.seniority, j.experience_min_years,
	(SELECT min(mp.first_seen_at) FROM jobs m JOIN postings mp ON mp.id = m.posting_id
	 WHERE m.company_id = g.company_id AND m.title_key = g.title_key)::timestamptz AS first_seen_at,
	cp.origin, cp.source
FROM job_groups g
JOIN companies c ON c.id = g.company_id
JOIN jobs j ON j.posting_id = g.canonical_posting_id
JOIN postings cp ON cp.id = g.canonical_posting_id
WHERE c.active OR EXISTS (
	SELECT 1 FROM jobs m JOIN postings mp ON mp.id = m.posting_id
	WHERE m.company_id = g.company_id AND m.title_key = g.title_key
		AND (mp.origin = 'manual' OR (mp.origin = 'board' AND NOT c.excluded))
);

-- name: ListGroupMembers :many
-- Every posting in the group of posting @id: the canonical one first, then by first seen.
SELECT m.posting_id, m.title, m.team, m.department, m.locations, m.workplace, m.url,
	p.first_seen_at, (p.closed_at IS NOT NULL)::boolean AS closed
FROM jobs j
JOIN jobs m ON m.company_id = j.company_id AND m.title_key = j.title_key
JOIN postings p ON p.id = m.posting_id
JOIN job_groups g ON g.company_id = j.company_id AND g.title_key = j.title_key
WHERE j.posting_id = @id AND j.title_key <> ''
ORDER BY m.posting_id = g.canonical_posting_id DESC, p.first_seen_at, m.posting_id;

-- name: CountJobLocations :many
-- How many jobs name each location string, most common first (M2-09 --report-unknown).
SELECT l::text AS location, count(*)::integer AS jobs
FROM jobs, unnest(locations) AS l
GROUP BY l
ORDER BY jobs DESC, l;
