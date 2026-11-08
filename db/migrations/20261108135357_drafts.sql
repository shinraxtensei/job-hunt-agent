-- +goose Up
-- M4-02, ADR-0025: tailored CV drafts. The API inserts a `requested` row (ADR-0022
-- guards); the worker queues a `tailor` item, writes the content, validates it in code
-- (M4-04) and stores it `blocked` or `ready_for_review`. Nothing is ever sent (guardrail
-- rule 6): the owner approves or discards. CV content: personal, never logged.
CREATE TABLE drafts (
	id               bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	posting_id       bigint      NOT NULL REFERENCES postings (id) ON DELETE CASCADE,
	status           text        NOT NULL DEFAULT 'requested' CHECK (status IN ('requested',
		'blocked', 'ready_for_review', 'approved', 'discarded', 'failed')),
	content_hash     bytea,      -- the posting content it was made for
	variant          text        CHECK (variant IN ('BE', 'PAY', 'DIST', 'FS', 'AI', 'CONS')),
	content          jsonb,      -- the rendered draft: header and facts from the bank, sentences from the model
	coverage         jsonb,      -- M4-03, as computed for this draft
	findings         jsonb,      -- M4-04 violations and warnings
	evidence_version text,
	profile_version  text,
	prompt_version   integer,
	model            text,
	error            text        CHECK (length(error) <= 500), -- why it failed, never CV text
	requested_at     timestamptz NOT NULL DEFAULT now(),
	made_at          timestamptz,
	decided_at       timestamptz -- approved or discarded
);

CREATE INDEX drafts_posting_idx ON drafts (posting_id, id DESC);

ALTER TABLE work_items DROP CONSTRAINT work_items_kind_check;
ALTER TABLE work_items ADD CONSTRAINT work_items_kind_check CHECK (kind IN ('extract', 'reason', 'tailor'));
