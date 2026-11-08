-- +goose Up
-- M6-05: why the owner skipped a lead (OI-16), with the score at that moment: M8's input.
-- One row per skip; the reason can be added just after the one-click skip.
CREATE TABLE skips (
	id             bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	application_id bigint      NOT NULL REFERENCES applications (id),
	reason         text        CHECK (reason IN ('location', 'seniority', 'stack', 'salary', 'company',
		'role_type', 'visa_relocation', 'language', 'not_interested', 'other')),
	note           text        CHECK (length(note) <= 500), -- the owner's words, never sent to a model
	score          jsonb,      -- {score, fit_level, breakdown} as shown when skipped
	at             timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX skips_app_idx ON skips (application_id, at);
