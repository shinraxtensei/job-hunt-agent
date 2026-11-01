# 0004. Use pgx, sqlc and goose for database access

- **Status:** Accepted
- **Date:** 2026-11-01
- **Story:** docs/stories/M0-03-local-postgres-and-migrations.md

## Context

PostgreSQL is the only shared state and the contract between Go and Python (ADR-0001).
Go needs a driver, a way to write queries without hand-mapping rows, and versioned
migrations that Python never has to know about. CLAUDE.md asks for standard library
first, so every non-stdlib dependency here needs a reason. Store tests must run against
real Postgres, not mocks.

## Decision

- **pgx v5** (`github.com/jackc/pgx/v5`) as the driver and pool. It is used natively by
  the store, and through its `database/sql` adapter by goose.
- **sqlc** generates typed Go from plain SQL in `internal/store/queries/*.sql`, using
  the migrations as the schema. The generated code is committed. sqlc runs from a pinned
  Docker image, not `tools.mod`, because its PostgreSQL parser needs cgo.
- **goose v3** (`github.com/pressly/goose/v3`) as a library inside `cmd/migrate`, with
  the SQL files embedded from `db/migrations/`. Forward-only: `cmd/migrate` exposes `up`
  and `status`, and no down commands.
- **testcontainers-go** (test-only) starts a throwaway Postgres from the same image
  pinned in `deploy/compose.yaml`.

Roles: migrations connect as `jobhunt_owner`, which owns the database. The app connects
as `jobhunt_app`, which has DML only, granted through default privileges. The one
exception is migration history (`goose_db_version`), which the app can read but not
write. Neither role is a superuser.

## Alternatives considered

- **database/sql + lib/pq:** lib/pq is in maintenance mode; pgx is faster and supports
  COPY, LISTEN/NOTIFY and native types (all useful for the queue).
- **An ORM (GORM, ent):** hides SQL, which is the contract we most need to see.
- **golang-migrate / Atlas:** golang-migrate wants paired down files; Atlas is heavier
  than a single-schema project needs. goose runs plain SQL and embeds cleanly.
- **Mocks or SQLite in tests:** they test a different database. Real Postgres via
  testcontainers costs a few seconds per run.

## Consequences

- **Good:** queries are plain SQL and checked at generation time; migrations ship inside
  the binary; tests exercise the real engine, roles and grants.
- **Bad:** `make sqlc` needs Docker; generated code has to be regenerated and committed
  whenever queries change; integration tests (and so `make check`) need Docker.
- **Revisit when:** sqlc can't express a query we need often, or schema changes need
  online migration tooling.
