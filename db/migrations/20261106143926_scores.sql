-- +goose Up
-- M3-06, ADR-0021: lead scores, written by the Python worker, read by the API (M3-08).
--
-- scores: the coded score of a shown lead's current extraction, one row per posting,
-- recomputed in place when its inputs change (no history). `points` is the weighted sum
-- without freshness; the freshness and bands parameters it was scored with are stored so
-- a reader (lead_scores) adds freshness at read time and never sees a stale total.
CREATE TABLE scores (
	posting_id                bigint           PRIMARY KEY REFERENCES postings (id) ON DELETE CASCADE,
	content_hash              bytea            NOT NULL, -- the posting content scored
	extraction_id             bigint           NOT NULL REFERENCES extractions (id) ON DELETE CASCADE,
	extraction_prompt_version integer          NOT NULL CHECK (extraction_prompt_version > 0),
	points                    double precision NOT NULL CHECK (points BETWEEN 0 AND 100),
	factors                   jsonb            NOT NULL, -- per factor: value 0..1, detail, skill matches
	not_enough_info           boolean          NOT NULL, -- no required skills stated
	freshness                 jsonb            NOT NULL, -- {weight, fresh_days, stale_days, unknown}
	bands                     jsonb            NOT NULL, -- {very_strong, strong, good, stretch}
	weights_version           text             NOT NULL,
	bands_version             text             NOT NULL,
	profile_version           text             NOT NULL,
	reason_inputs             text             NOT NULL, -- what a current reason was made from
	reason_requested_at       timestamptz,               -- set when the lead is opened (M3-08)
	scored_at                 timestamptz      NOT NULL DEFAULT now()
);

-- The model-written reason, gaps and CV variant, regenerated lazily (M3-06 AC5): at once
-- for new posting content, otherwise only when requested. Untrusted when displayed.
CREATE TABLE score_reasons (
	posting_id     bigint      PRIMARY KEY REFERENCES postings (id) ON DELETE CASCADE,
	content_hash   bytea       NOT NULL, -- the posting content it describes
	inputs         text        NOT NULL, -- scores.reason_inputs it was made from
	prompt_version integer     NOT NULL CHECK (prompt_version > 0),
	model          text        NOT NULL,
	reason         text        NOT NULL CHECK (length(reason) BETWEEN 1 AND 610),
	gaps           jsonb       NOT NULL, -- skill names from the breakdown, never invented
	cv_variant     text        NOT NULL CHECK (cv_variant IN ('BE', 'PAY', 'DIST', 'FS', 'AI', 'CONS')),
	created_at     timestamptz NOT NULL DEFAULT now()
);

-- The worker enqueues its own `reason` items (ADR-0021); Go still enqueues `extract`.
ALTER TABLE work_items DROP CONSTRAINT work_items_kind_check;
ALTER TABLE work_items ADD CONSTRAINT work_items_kind_check CHECK (kind IN ('extract', 'reason'));

-- What a reader shows: the total with freshness as of now, the fit level from the stored
-- bands, and whether the reason is from older inputs (still shown, marked).
CREATE VIEW lead_scores AS
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
	SELECT s.points + (s.freshness ->> 'weight')::float8 * CASE
		WHEN j.published_at IS NULL THEN (s.freshness ->> 'unknown')::float8
		WHEN extract(epoch FROM now() - j.published_at) / 86400 <= (s.freshness ->> 'fresh_days')::float8 THEN 1
		ELSE greatest(0, ((s.freshness ->> 'stale_days')::float8 - extract(epoch FROM now() - j.published_at) / 86400)
			/ ((s.freshness ->> 'stale_days')::float8 - (s.freshness ->> 'fresh_days')::float8))
	END AS total
) t;
