# 0006. Record collector runs in Postgres from M1-02; goleak in tests

- **Status:** Accepted
- **Date:** 2026-11-01
- **Story:** docs/stories/M1-02-concurrent-collector-run.md

## Context

M1-02 makes `collector run` fetch every company concurrently. A failed company must be
"recorded with its error" and the run must end with an outcome (ok / partial). M1-08
(schedule and monitoring) already specifies a `collector_runs` table, but it comes later;
until then a run's result would exist only in logs, which nobody queries and which
rotate away. The tests also need to prove no goroutine outlives a run (AC6), and the
standard library has no leak check.

## Decision

- Create **`collector_runs`** now, with the columns M1-08 AC2 lists: started_at,
  ended_at, outcome, companies attempted / failed, postings new / seen (already stored,
  re-checked; M1-07 adds postings_changed for real payload changes). Plus
  `companies_skipped` (boards with no adapter, AC5). Outcomes: `ok`, `partial`, `failed`,
  and `cancelled` for a run stopped by SIGINT/SIGTERM. A row with no `ended_at` is a run
  that died without finishing.
- Create **`collector_run_errors`** (run_id, company_id, error): one row per failed
  company, so "which boards broke last night" is a query. Error text is our own wrapped
  error (status and company, never response bodies), capped at 1,000 characters.
- Both are written by the app role. M1-08 then only adds the schedule and the
  healthchecks.io pings.
- **`go.uber.org/goleak`** (test-only) checks every run test for leaked goroutines.

## Alternatives considered

- **Logs only until M1-08:** the failure is printed, not recorded; M1-08 would start with
  no history and a migration that has to guess at old runs.
- **A JSON summary file per run:** a second store to back up and query; Postgres is the
  only shared state (ADR-0001).
- **Counting goroutines with `runtime.NumGoroutine`:** flaky with background goroutines
  from pgx and testcontainers; goleak snapshots and diffs them properly.

## Consequences

- **Good:** every run is queryable from day one; M1-08 shrinks to scheduling and alerting;
  leaks fail the test that caused them.
- **Bad:** two more tables in the shared schema; one more test dependency.
- **Revisit when:** run history needs per-company timings or postings counts (add
  columns), or the table grows large enough to need pruning (one row a day won't).
