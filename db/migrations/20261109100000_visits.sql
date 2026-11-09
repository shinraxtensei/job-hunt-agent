-- +goose Up
-- M7-01 (OI-20): the owner's visits to the Today view, so it can show what changed since
-- the last one. A visit lasts while the page is opened again within 30 minutes; "Mark all
-- seen" stamps the current one. One owner (ADR-0002): no user column.
CREATE TABLE visits (
	id           bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	started_at   timestamptz NOT NULL,
	last_seen_at timestamptz NOT NULL,
	marked_at    timestamptz -- "Mark all seen": nothing before this is new
);
CREATE INDEX visits_started_idx ON visits (started_at DESC);
