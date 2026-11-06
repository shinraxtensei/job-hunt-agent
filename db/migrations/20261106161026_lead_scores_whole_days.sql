-- +goose Up
-- M3-08: freshness counts whole days since published, as score.py and the API do, so a
-- lead's total (and the score sort) only changes at a day boundary.
CREATE OR REPLACE VIEW lead_scores AS
SELECT
	s.posting_id,
	t.total,
	CASE
		WHEN t.total >= (s.bands ->> 'very_strong')::float8 THEN 'very_strong'
		WHEN t.total >= (s.bands ->> 'strong')::float8 THEN 'strong'
		WHEN t.total >= (s.bands ->> 'good')::float8 THEN 'good'
		WHEN t.total >= (s.bands ->> 'stretch')::float8 THEN 'stretch'
		ELSE 'no'
	END AS fit_level,
	s.not_enough_info,
	s.factors,
	r.reason,
	r.gaps,
	r.cv_variant,
	r.posting_id IS NOT NULL AND r.content_hash = s.content_hash AND r.inputs = s.reason_inputs
		AS reason_current
FROM scores s
JOIN jobs j ON j.posting_id = s.posting_id
LEFT JOIN score_reasons r ON r.posting_id = s.posting_id
CROSS JOIN LATERAL (
	SELECT floor(extract(epoch FROM now() - j.published_at) / 86400) AS age
) a
CROSS JOIN LATERAL (
	SELECT s.points + (s.freshness ->> 'weight')::float8 * CASE
		WHEN j.published_at IS NULL THEN (s.freshness ->> 'unknown')::float8
		WHEN a.age <= (s.freshness ->> 'fresh_days')::float8 THEN 1
		ELSE greatest(0, ((s.freshness ->> 'stale_days')::float8 - a.age)
			/ ((s.freshness ->> 'stale_days')::float8 - (s.freshness ->> 'fresh_days')::float8))
	END AS total
) t;
