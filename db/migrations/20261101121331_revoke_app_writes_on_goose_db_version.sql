-- +goose Up
-- goose creates goose_db_version as the owner, so the default privileges in
-- deploy/postgres/initdb/01-roles.sh handed the app DML on it. Migration history is
-- the owner's alone; the app may still read it.
REVOKE INSERT, UPDATE, DELETE ON goose_db_version FROM jobhunt_app;
