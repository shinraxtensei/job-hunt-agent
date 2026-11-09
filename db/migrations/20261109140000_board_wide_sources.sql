-- +goose Up
-- ADR-0029: board-wide sources (M7-03). Remotive is a board type; its postings belong to
-- many companies and enter with origin 'board'.
ALTER TABLE companies DROP CONSTRAINT companies_board_type_check,
	ADD CONSTRAINT companies_board_type_check
		CHECK (board_type IN ('lever', 'greenhouse', 'ashby', 'workable', 'personio', 'remotive')),
	-- the last successful fetch, for sources with a minimum fetch interval
	ADD COLUMN fetched_at timestamptz;

ALTER TABLE postings DROP CONSTRAINT postings_origin_check,
	ADD CONSTRAINT postings_origin_check CHECK (origin IN ('collected', 'manual', 'board'));

-- job_groups (ADR-0009, ADR-0010): the canonical member is open first, then a company's
-- own posting before a board's, then the one seen first. Columns unchanged.
CREATE OR REPLACE VIEW job_groups AS
SELECT
	j.company_id,
	j.title_key,
	count(*)::integer AS members,
	(array_agg(j.posting_id
		ORDER BY (p.closed_at IS NOT NULL), (p.origin = 'board'), p.first_seen_at, j.posting_id))[1]
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
