-- +goose Up
-- ADR-0032 (M9-05): an application that ended (rejected or closed) can be set aside, so the
-- lead can be applied to again. A group has one current application (archived_at IS NULL)
-- and any number of earlier ones, kept with their history.
ALTER TABLE applications ADD COLUMN archived_at timestamptz;
ALTER TABLE applications DROP CONSTRAINT applications_company_id_title_key_key;
CREATE UNIQUE INDEX applications_current ON applications (company_id, title_key) WHERE archived_at IS NULL;
