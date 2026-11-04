-- +goose Up
-- M3-03, ADR-0018: one row per model call, written by the Python worker. Never prompt or
-- response text. Today's (UTC) sum of cost_usd is the daily budget's spend.
CREATE TABLE model_calls (
	id                          bigint        GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	created_at                  timestamptz   NOT NULL DEFAULT now(),
	agent                       text          NOT NULL, -- extract, score, decide, ...
	prompt_file                 text          NOT NULL, -- e.g. decide/v1.md
	prompt_version              integer       NOT NULL CHECK (prompt_version > 0),
	model                       text          NOT NULL, -- the exact model id
	input_tokens                integer       NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
	output_tokens               integer       NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
	cache_read_input_tokens     integer       NOT NULL DEFAULT 0 CHECK (cache_read_input_tokens >= 0),
	cache_creation_input_tokens integer       NOT NULL DEFAULT 0 CHECK (cache_creation_input_tokens >= 0),
	cost_usd                    numeric(12,6) NOT NULL DEFAULT 0 CHECK (cost_usd >= 0),
	latency_ms                  integer       NOT NULL CHECK (latency_ms >= 0),
	-- ok; invalid (schema-invalid output); refusal; max_tokens (truncated); error (no answer)
	outcome                     text          NOT NULL CHECK (outcome IN ('ok', 'invalid', 'refusal', 'max_tokens', 'error')),
	posting_id                  bigint        REFERENCES postings (id) ON DELETE SET NULL
);

CREATE INDEX model_calls_created_at ON model_calls (created_at);
