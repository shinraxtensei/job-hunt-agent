-- +goose Up
-- ADR-0005: the watch list (data/companies.yaml) syncs into companies.
ALTER TABLE companies
	-- NULL only for rows the collector creates ad hoc, outside the watch list.
	ADD COLUMN track text CHECK (track IN ('morocco', 'remote', 'relocation', 'consulting')),
	-- false once a company leaves the watch list; rows are never deleted.
	ADD COLUMN active boolean NOT NULL DEFAULT true,
	ADD CONSTRAINT companies_board_type_check
		CHECK (board_type IN ('lever', 'greenhouse', 'ashby', 'workable')),
	ADD CONSTRAINT companies_board_key UNIQUE (board_type, board_slug);
