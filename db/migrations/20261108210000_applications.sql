-- +goose Up
-- M6-01, ADR-0028: an application per duplicate group (company_id, title_key: the lead),
-- created when the lead first leaves `new`. Its status history is append-only. When it is
-- first `applied`, the posting as applied to is kept (postings keep only the latest
-- payload, ADR-0007). Posting text in the snapshot is untrusted: never logged.
CREATE TABLE applications (
	id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	company_id  bigint      NOT NULL REFERENCES companies (id),
	title_key   text        NOT NULL,
	posting_id  bigint      NOT NULL REFERENCES postings (id), -- the lead's posting when created
	status      text        NOT NULL CHECK (status IN ('new', 'shortlisted', 'drafted', 'applied',
		'replied', 'interviewing', 'offer', 'rejected', 'closed', 'skipped')),
	status_at   timestamptz NOT NULL DEFAULT now(),
	applied_at  timestamptz,          -- first time it was applied
	snapshot    jsonb,                -- {title, url, raw_payload, content_hash} when first applied
	created_at  timestamptz NOT NULL DEFAULT now(),
	UNIQUE (company_id, title_key)
);

CREATE TABLE application_events (
	id             bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	application_id bigint      NOT NULL REFERENCES applications (id),
	status         text        NOT NULL,
	at             timestamptz NOT NULL DEFAULT now(),
	auto           boolean     NOT NULL DEFAULT false -- set by the system (OI-11), not the owner
);
CREATE INDEX application_events_app_idx ON application_events (application_id, at);
