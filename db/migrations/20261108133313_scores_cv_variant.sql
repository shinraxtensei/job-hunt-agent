-- +goose Up
-- M4-06: the CV variant is chosen in code with the score (variant.py), not by the
-- reason's decide() call. NULL until the lead is re-scored.
ALTER TABLE scores ADD COLUMN cv_variant text CHECK (cv_variant IN ('BE', 'PAY', 'DIST', 'FS', 'AI', 'CONS'));
