-- +goose Up
-- ADR-0007: know when a posting's payload changed, not just when it was last seen.
ALTER TABLE postings
	-- SHA-256 of the canonical jsonb text, computed by Postgres in the upsert.
	ADD COLUMN content_hash bytea,
	-- When the payload last changed; equals first_seen_at until the first change.
	ADD COLUMN last_changed_at timestamptz;

UPDATE postings
SET content_hash = sha256(convert_to(raw_payload::text, 'UTF8')),
	last_changed_at = first_seen_at;

ALTER TABLE postings
	ALTER COLUMN content_hash SET NOT NULL,
	ALTER COLUMN last_changed_at SET NOT NULL,
	ALTER COLUMN last_changed_at SET DEFAULT now();

-- Postings re-checked whose payload changed; postings_seen counts the unchanged ones.
ALTER TABLE collector_runs ADD COLUMN postings_changed integer NOT NULL DEFAULT 0;
