-- +goose Up
-- M5-01: the cover letter, made with the CV in the same draft (ADR-0025 amendment), and
-- the evidence bank's `letter` lines (owner-written, letter only; OI-6).
ALTER TABLE drafts ADD COLUMN letter jsonb; -- greeting and opening by code, sentences from the model

ALTER TABLE evidence_items DROP CONSTRAINT evidence_items_kind_check,
	ADD CONSTRAINT evidence_items_kind_check CHECK (kind IN ('person', 'summary', 'role',
		'project', 'education', 'certification', 'languages', 'letter'));

ALTER TABLE evidence_lines DROP CONSTRAINT evidence_lines_kind_check,
	ADD CONSTRAINT evidence_lines_kind_check CHECK (kind IN ('summary', 'role', 'project',
		'letter'));
