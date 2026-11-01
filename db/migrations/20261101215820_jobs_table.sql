-- +goose Up
-- ADR-0008: the normalised job record, one row per posting, rebuildable from raw_payload.
CREATE TABLE jobs (
	posting_id         bigint PRIMARY KEY REFERENCES postings (id) ON DELETE CASCADE,
	company_id         bigint      NOT NULL REFERENCES companies (id),
	title              text        NOT NULL,
	url                text        NOT NULL,
	locations          text[]      NOT NULL DEFAULT '{}',
	country_codes      text[]      NOT NULL DEFAULT '{}', -- ISO alpha-2, only where the board gives codes
	workplace          text        NOT NULL CHECK (workplace IN ('remote', 'hybrid', 'onsite', 'unknown')),
	team               text        NOT NULL DEFAULT '',
	department         text        NOT NULL DEFAULT '',
	employment_type    text        NOT NULL DEFAULT '', -- as published
	published_at       timestamptz,                     -- NULL: the board doesn't say
	normaliser_version integer     NOT NULL,
	posting_hash       bytea       NOT NULL,            -- the posting's content_hash this row was made from
	normalised_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX jobs_company_id_idx ON jobs (company_id);

-- Postings in a run that could not be normalised (still stored; no job record).
ALTER TABLE collector_runs ADD COLUMN jobs_failed integer NOT NULL DEFAULT 0;
