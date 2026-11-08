-- +goose Up
-- M4-01, ADR-0024: the evidence bank, synced by the worker from data/profile/evidence.yaml.
-- Personal data: on the owner's machine only. Rows missing from the file are retired,
-- never deleted, so drafts citing them stay checkable.

-- The version in the database (sha256 of the file and the skill map's version).
CREATE TABLE evidence_bank (
	id        boolean     PRIMARY KEY DEFAULT true CHECK (id), -- one row
	version   text        NOT NULL,
	loaded_at timestamptz NOT NULL DEFAULT now()
);

-- The person (header), the summary, roles, projects, education, certifications and the
-- languages list. `facts` are printed exactly (titles, employers, dates, degrees).
CREATE TABLE evidence_items (
	id         text        PRIMARY KEY,
	kind       text        NOT NULL CHECK (kind IN ('person', 'summary', 'role', 'project',
		'education', 'certification', 'languages')),
	facts      jsonb       NOT NULL,
	position   integer     NOT NULL, -- order in the file
	retired    boolean     NOT NULL DEFAULT false,
	version    text        NOT NULL, -- the bank version it was last seen in
	updated_at timestamptz NOT NULL DEFAULT now()
);

-- One row per evidence line: what a draft bullet cites.
CREATE TABLE evidence_lines (
	id         text        PRIMARY KEY,
	item_id    text        NOT NULL REFERENCES evidence_items (id),
	kind       text        NOT NULL CHECK (kind IN ('summary', 'role', 'project')),
	text       text        NOT NULL,
	variants   text[]      NOT NULL,
	skills     jsonb       NOT NULL, -- [{name, skill, level}]: levels when the line was loaded
	numbers    jsonb       NOT NULL, -- [{value, unit, approx, plus}] (numbers.py)
	source     text,                 -- where a drafted line came from (cv, skill_map: …)
	position   integer     NOT NULL,
	retired    boolean     NOT NULL DEFAULT false,
	version    text        NOT NULL,
	updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX evidence_lines_item_idx ON evidence_lines (item_id);
