-- +goose Up
-- ADR-0015: manually added postings. Companies typed with --company have no board.
ALTER TABLE companies
	ALTER COLUMN board_type DROP NOT NULL,
	ALTER COLUMN board_slug DROP NOT NULL,
	ADD CONSTRAINT companies_board_both CHECK ((board_type IS NULL) = (board_slug IS NULL));
-- Board-less companies are one row per name, whatever the case.
CREATE UNIQUE INDEX companies_boardless_name ON companies (lower(name)) WHERE board_type IS NULL;

ALTER TABLE postings
	-- How the posting entered: collected from a watched board, or added by link.
	ADD COLUMN origin text NOT NULL DEFAULT 'collected' CHECK (origin IN ('collected', 'manual')),
	-- Manual board postings are rechecked one by one (NULL: never).
	ADD COLUMN last_rechecked_at timestamptz;

ALTER TABLE collector_runs ADD COLUMN manual_rechecked integer NOT NULL DEFAULT 0;
