# M1-02: Concurrent collector run

- **Status:** Ready
- **Milestone:** M1 — Collector
- **Linear:** [HAM-21](https://linear.app/hamid-corp/issue/HAM-21/m1-02-concurrent-collector-run)
- **Estimate:** 1 day

## Story

As the job seeker, I want one collector run to fetch every active company concurrently
and politely, so that 50 companies finish in minutes and one broken board never stops the
rest.

## Context

Introduces the `Source` interface (defined in `internal/collector`, the consumer) and the
source registry. Follows the `source-adapter` skill's politeness rules.

## Acceptance criteria

1. **Given** active companies across several board types
   **When** `collector run` executes
   **Then** companies are fetched concurrently (bounded by `COLLECTOR_CONCURRENCY`,
   default 8) using `errgroup` and a shared `context`.
2. **Given** several companies on the same host
   **When** they are fetched
   **Then** requests to that host never exceed its configured rate (default 1/s), verified
   by a test with a fake source counting request times.
3. **Given** one company's fetch fails
   **When** the run finishes
   **Then** the other companies' results are still stored, the failure is recorded with
   its error, and the run exits 0 with a "partial" outcome.
4. **Given** SIGINT or SIGTERM during a run
   **When** received
   **Then** in-flight requests are cancelled, nothing half-written remains, and the
   process exits within 10 seconds.
5. **Given** a board type with no registered adapter
   **When** the run starts
   **Then** those companies are skipped with a warning naming them.
6. **Given** the test suite
   **When** run with `-race` and goleak
   **Then** no data races and no leaked goroutines are reported.

## Out of scope

- Scheduling (M1-08), the adapters themselves (M1-03 to M1-06).

## Security notes

Every HTTP client has a timeout and a descriptive User-Agent with a contact URL.
