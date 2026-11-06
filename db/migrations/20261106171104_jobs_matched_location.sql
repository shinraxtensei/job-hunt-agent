-- +goose Up
-- ADR-0023: the location that passed the default preset's location rules (M2-08's
-- matched_location), written by the reconcile step for the canonical posting of each
-- shown group, read by the scorer for the location factor's detail and the reason
-- ("on-site in London", not the job's first location). '' when none applied.
ALTER TABLE jobs ADD COLUMN matched_location text NOT NULL DEFAULT '';
