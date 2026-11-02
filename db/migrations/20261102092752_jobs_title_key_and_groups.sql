-- +goose Up
-- ADR-0009: duplicate groups are jobs with the same (company_id, title_key).
-- '' until `collector normalise` (normaliser version 2) fills it for existing rows.
ALTER TABLE jobs ADD COLUMN title_key text NOT NULL DEFAULT '';
CREATE INDEX jobs_group_idx ON jobs (company_id, title_key);

-- One row per duplicate group: the canonical posting (seen first; M2-04 will prefer open
-- ones) and the union of members' locations and workplaces. Derived on read, never stale.
CREATE VIEW job_groups AS
SELECT
	j.company_id,
	j.title_key,
	count(*)::integer AS members,
	(array_agg(j.posting_id ORDER BY p.first_seen_at, j.posting_id))[1] AS canonical_posting_id,
	ARRAY(
		SELECT DISTINCT l FROM jobs m, unnest(m.locations) AS l
		WHERE m.company_id = j.company_id AND m.title_key = j.title_key
		ORDER BY l
	) AS locations,
	ARRAY(
		SELECT DISTINCT m.workplace FROM jobs m
		WHERE m.company_id = j.company_id AND m.title_key = j.title_key
		ORDER BY m.workplace
	) AS workplaces
FROM jobs j
JOIN postings p ON p.id = j.posting_id
WHERE j.title_key <> ''
GROUP BY j.company_id, j.title_key;
