-- +goose Up
-- M7-03: Remote OK, a second board-wide source (ADR-0029).
ALTER TABLE companies DROP CONSTRAINT companies_board_type_check,
	ADD CONSTRAINT companies_board_type_check
		CHECK (board_type IN ('lever', 'greenhouse', 'ashby', 'workable', 'personio', 'remotive', 'remoteok'));
