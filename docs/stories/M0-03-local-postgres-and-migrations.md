# M0-03: Local Postgres and migrations

- **Status:** Ready
- **Milestone:** M0 — Walking skeleton
- **Linear:** [HAM-17](https://linear.app/hamid-corp/issue/HAM-17/m0-03-local-postgres-and-migrations)
- **Estimate:** ½ day

## Story

As the job seeker building this tool, I want a local database with versioned migrations,
so that the schema is reproducible on my laptop, in tests and on the server.

## Context

PostgreSQL is the only shared state (ADR-0001). Migrations are plain SQL, forward-only,
run by goose from `cmd/migrate`.

## Acceptance criteria

1. **Given** Docker running
   **When** I run `make up`
   **Then** PostgreSQL (current major version) starts from `deploy/compose.yaml` with a
   named volume, and `make down` stops it without deleting data.
2. **Given** an empty database
   **When** I run `go run ./cmd/migrate up`
   **Then** all migrations apply in order, and running it again is a no-op.
3. **Given** the first migration
   **When** applied
   **Then** tables `companies` (id, name, board_type, board_slug, excluded, created_at)
   and `postings` (id, company_id, source, source_posting_id, url, title, raw_payload
   jsonb, first_seen_at, last_checked_at) exist, with a unique constraint on
   (source, source_posting_id).
4. **Given** `make migration name=add_x`
   **When** run
   **Then** a new timestamped SQL file is created in `db/migrations/`.
5. **Given** the store package
   **When** integration tests run (`make test-integration`)
   **Then** a throwaway Postgres starts via testcontainers-go, migrations apply, and a
   test inserts and reads a company.

## Out of scope

- sqlc query generation beyond one smoke query (grows in M1), the work queue table (M3).

## Security notes

Local credentials only in `.env` (gitignored); commit a `.env.example` with placeholders.
The app connects with a non-superuser role; only migrations use the owner role.
