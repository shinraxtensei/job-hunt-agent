-- +goose Up
-- M6-03: the owner's dated notes and the people on each application. Contacts are other
-- people's personal data (OI-14): only here and in backups; never logged, never sent to
-- a model, never exported.
CREATE TABLE notes (
	id             bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	application_id bigint      NOT NULL REFERENCES applications (id),
	body           text        NOT NULL CHECK (length(body) BETWEEN 1 AND 5000),
	created_at     timestamptz NOT NULL DEFAULT now(),
	edited_at      timestamptz
);
CREATE INDEX notes_app_idx ON notes (application_id);

CREATE TABLE contacts (
	id             bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	application_id bigint      NOT NULL REFERENCES applications (id),
	name           text        NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
	role           text        NOT NULL DEFAULT '' CHECK (length(role) <= 200),
	email          text        NOT NULL DEFAULT '' CHECK (length(email) <= 254),
	linkedin       text        NOT NULL DEFAULT '' CHECK (length(linkedin) <= 300),
	phone          text        NOT NULL DEFAULT '' CHECK (length(phone) <= 40),
	created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX contacts_app_idx ON contacts (application_id);
