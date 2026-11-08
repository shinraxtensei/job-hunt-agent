-- +goose Up
-- M6-04: follow-up reminders 7 days after applying (OI-15). Done and snoozed are kept on
-- the application; each action is also a history entry (its note), so nothing is lost.
ALTER TABLE applications
	ADD COLUMN followup_done_at timestamptz,
	ADD COLUMN followup_snoozed_until timestamptz;
ALTER TABLE application_events ADD COLUMN note text CHECK (length(note) <= 200); -- e.g. "follow-up snoozed 3 days"
