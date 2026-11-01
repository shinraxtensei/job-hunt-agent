# 0005. Companies table: track, active, one row per board, YAML watch list

- **Status:** Accepted
- **Date:** 2026-11-01
- **Story:** docs/stories/M1-01-watch-list-seed-and-companies-sync.md

## Context

The watch list moves into a reviewed file (`data/companies.yaml`) that syncs to
`companies`. `companies` is shared state that the Python agents will read for scoring
(track decides which rules and CV angle apply). Sync must be idempotent and must never
delete: postings reference companies, and a company dropped from the file may return.
Until now nothing stopped two rows for the same board (M0-04 left a `ponytail:` note),
which would split one company's postings across rows.

## Decision

Migration on `companies`:
- `track text NULL CHECK (track IN ('morocco','remote','relocation','consulting'))`.
  NULL only for rows the collector creates ad hoc (`COLLECTOR_LEVER_COMPANY`), which
  are not on the watch list.
- `active boolean NOT NULL DEFAULT true`. Sync sets it false for rows no longer in the
  file; rows are never deleted. `excluded` stays separate: an excluded company is on
  the list on purpose (so it can't be re-added by accident) and is never collected.
- `UNIQUE (board_type, board_slug)`: the natural key sync and the collector upsert on.
- `CHECK (board_type IN ('lever','greenhouse','ashby','workable'))`: the boards we
  have or plan adapters for. A new board needs a migration, as the source-adapter skill
  says.

The file is parsed with `go.yaml.in/yaml/v3` (the YAML org's maintained successor to
`gopkg.in/yaml.v3`, already in `go.sum` through testcontainers). Its node API gives line
numbers, so validation can list every invalid line.

## Alternatives considered

- **Postgres enum types** for track and board_type: adding a value later is an
  `ALTER TYPE` that sqlc and Python both have to follow; a CHECK is simpler to change.
- **Delete rows removed from the file:** breaks posting history and foreign keys.
- **JSON or TOML for the file:** JSON has no comments (the list needs them); TOML has no
  stdlib parser either, so it's no cheaper than YAML and less familiar.
- **Seed with a SQL migration:** the list changes often; migrations are forward-only and
  not meant for data the owner edits.

## Consequences

- **Good:** one row per board; sync and the collector can both upsert; Python can rely
  on track values; history survives removals.
- **Bad:** a new track or board type needs a migration; one more direct dependency.
- **Revisit when:** a company needs more than one board, or tracks become per-posting
  rather than per-company.
