-- +goose Up
-- ADR-0006: one row per collector run (M1-08 AC2's columns) and one per failed company.
CREATE TABLE collector_runs (
	id                  bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	started_at          timestamptz NOT NULL DEFAULT now(),
	ended_at            timestamptz, -- NULL: the run died without finishing
	outcome             text CHECK (outcome IN ('ok', 'partial', 'failed', 'cancelled')),
	companies_attempted integer     NOT NULL DEFAULT 0,
	companies_failed    integer     NOT NULL DEFAULT 0,
	companies_skipped   integer     NOT NULL DEFAULT 0, -- board type has no adapter
	postings_new        integer     NOT NULL DEFAULT 0,
	postings_seen       integer     NOT NULL DEFAULT 0  -- already stored; only last_checked_at moved
	-- (M1-07 adds postings_changed for postings whose payload actually changed)
);

CREATE TABLE collector_run_errors (
	run_id     bigint NOT NULL REFERENCES collector_runs (id),
	company_id bigint NOT NULL REFERENCES companies (id),
	error      text   NOT NULL,
	PRIMARY KEY (run_id, company_id)
);
