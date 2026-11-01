-- +goose Up
CREATE TABLE companies (
	id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	name       text        NOT NULL,
	board_type text        NOT NULL,
	board_slug text        NOT NULL,
	excluded   boolean     NOT NULL DEFAULT false,
	created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE postings (
	id                bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	company_id        bigint      NOT NULL REFERENCES companies (id),
	source            text        NOT NULL,
	source_posting_id text        NOT NULL,
	url               text        NOT NULL,
	title             text        NOT NULL,
	raw_payload       jsonb       NOT NULL,
	first_seen_at     timestamptz NOT NULL DEFAULT now(),
	last_checked_at   timestamptz NOT NULL DEFAULT now(),
	UNIQUE (source, source_posting_id)
);

CREATE INDEX postings_company_id_idx ON postings (company_id);
