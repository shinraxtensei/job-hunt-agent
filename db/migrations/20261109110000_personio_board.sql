-- +goose Up
-- M7-02: Personio career feeds (OI-17) join the watch list's board types.
ALTER TABLE companies DROP CONSTRAINT companies_board_type_check,
	ADD CONSTRAINT companies_board_type_check CHECK (board_type IN ('lever', 'greenhouse', 'ashby', 'workable', 'personio'));
