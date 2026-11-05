-- +goose Up
-- M3-04, ADR-0020: extraction results, written by the Python worker, read by Go
-- (`collector enqueue --reextract`) and later the API (M3-08).
--
-- One row per (posting, posting content, prompt version): the same content with the same
-- prompt is never extracted twice (AC4); a new prompt version adds a row, the old ones
-- stay for comparison. `data` is the validated Extraction (untrusted when shown).
CREATE TABLE extractions (
	id             bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	posting_id     bigint      NOT NULL REFERENCES postings (id) ON DELETE CASCADE,
	content_hash   bytea       NOT NULL, -- the posting content extracted
	prompt_version integer     NOT NULL CHECK (prompt_version > 0),
	model          text        NOT NULL, -- the exact model id
	data           jsonb       NOT NULL,
	created_at     timestamptz NOT NULL DEFAULT now(),
	UNIQUE (posting_id, content_hash, prompt_version)
);

-- The prompt version each agent runs now, written by the worker at start-up. Go reads it
-- to find extractions made with an older prompt (AC7); it never knows prompt files.
CREATE TABLE agent_prompts (
	agent      text        PRIMARY KEY,
	version    integer     NOT NULL CHECK (version > 0),
	updated_at timestamptz NOT NULL DEFAULT now()
);

