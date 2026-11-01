#!/bin/sh
# Runs once, on first start with an empty data directory (docker-entrypoint-initdb.d).
# jobhunt_owner owns the database and runs migrations; jobhunt_app gets DML only.
# Neither is a superuser.
set -eu

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
	-v db="$POSTGRES_DB" \
	-v owner_pw="$JOBHUNT_OWNER_PASSWORD" \
	-v app_pw="$JOBHUNT_APP_PASSWORD" <<'SQL'
CREATE ROLE jobhunt_owner LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE PASSWORD :'owner_pw';
CREATE ROLE jobhunt_app   LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE PASSWORD :'app_pw';

ALTER DATABASE :"db" OWNER TO jobhunt_owner;
ALTER SCHEMA public OWNER TO jobhunt_owner;
REVOKE ALL ON DATABASE :"db" FROM PUBLIC;
GRANT CONNECT ON DATABASE :"db" TO jobhunt_app;
GRANT USAGE ON SCHEMA public TO jobhunt_app;

-- Every table and sequence the owner creates (i.e. every migration) is usable by the app.
ALTER DEFAULT PRIVILEGES FOR ROLE jobhunt_owner IN SCHEMA public
	GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO jobhunt_app;
ALTER DEFAULT PRIVILEGES FOR ROLE jobhunt_owner IN SCHEMA public
	GRANT USAGE, SELECT ON SEQUENCES TO jobhunt_app;
SQL
