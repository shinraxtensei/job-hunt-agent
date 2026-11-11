-- +goose Up
-- ADR-0030 (M9-02): which group member passed the default preset's location rules, so the
-- scorer reads that member's place, not the canonical posting's. NULL: none applied.
ALTER TABLE jobs ADD COLUMN matched_posting_id bigint REFERENCES postings (id);
