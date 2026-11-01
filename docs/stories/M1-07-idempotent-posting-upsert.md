# M1-07: Idempotent posting upsert

- **Status:** Ready
- **Milestone:** M1 — Collector
- **Linear:** [HAM-26](https://linear.app/hamid-corp/issue/HAM-26/m1-07-idempotent-posting-upsert)
- **Estimate:** ½ day

## Story

As the job seeker, I want each posting stored once with its full history of sightings,
so that reruns are safe and I know when a role first appeared and was last seen.

## Context

Generalises the M0-04 insert into the store layer (pgx + sqlc). Adds `content_hash` and
`last_changed_at` to `postings` in a new migration. Raw text is always kept
so extraction can be rerun later without refetching.

## Acceptance criteria

1. **Given** a batch of raw postings for one company
   **When** stored
   **Then** it happens in one transaction using `INSERT … ON CONFLICT (source,
   source_posting_id) DO UPDATE`, returning counts of new and updated rows.
2. **Given** a posting already stored whose payload changed
   **When** stored again
   **Then** `raw_payload` and `content_hash` update, `last_changed_at` is set, and
   `first_seen_at` is unchanged.
3. **Given** a posting already stored with an identical payload
   **When** stored again
   **Then** only `last_checked_at` changes.
4. **Given** the integration tests (testcontainers)
   **When** they run
   **Then** cases 1–3 are covered against real Postgres, not mocks.

## Out of scope

- Detecting closed postings (M2-04), deduplication across sources (M2-02).

## Security notes

All SQL through sqlc-generated, parameterised queries.
