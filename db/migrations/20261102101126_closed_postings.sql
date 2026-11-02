-- +goose Up
-- ADR-0010: postings missing from a successful full-board fetch are closed, never deleted.
ALTER TABLE postings ADD COLUMN closed_at timestamptz; -- NULL: open
ALTER TABLE collector_runs
	ADD COLUMN postings_closed integer NOT NULL DEFAULT 0,
	ADD COLUMN postings_reopened integer NOT NULL DEFAULT 0;

-- job_groups (ADR-0009): the canonical member is the open one seen first; a group is
-- closed only when every member is. Columns are unchanged except `closed`, added last.
CREATE OR REPLACE VIEW job_groups AS
SELECT
	j.company_id,
	j.title_key,
	count(*)::integer AS members,
	(array_agg(j.posting_id ORDER BY (p.closed_at IS NOT NULL), p.first_seen_at, j.posting_id))[1]
		AS canonical_posting_id,
	ARRAY(
		SELECT DISTINCT l FROM jobs m, unnest(m.locations) AS l
		WHERE m.company_id = j.company_id AND m.title_key = j.title_key
		ORDER BY l
	) AS locations,
	ARRAY(
		SELECT DISTINCT m.workplace FROM jobs m
		WHERE m.company_id = j.company_id AND m.title_key = j.title_key
		ORDER BY m.workplace
	) AS workplaces,
	bool_and(p.closed_at IS NOT NULL) AS closed
FROM jobs j
JOIN postings p ON p.id = j.posting_id
WHERE j.title_key <> ''
GROUP BY j.company_id, j.title_key;
