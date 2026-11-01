# M1-08: Daily schedule and run monitoring

- **Status:** Ready
- **Milestone:** M1 — Collector
- **Linear:** [HAM-27](https://linear.app/hamid-corp/issue/HAM-27/m1-08-daily-schedule-and-run-monitoring)
- **Estimate:** ½ day

## Story

As the job seeker, I want the collector to run every morning on its own and tell me when
it doesn't, so that I never trust stale data without knowing.

## Context

Runs on the server. Keep it simple: a scheduled container (or systemd timer) invoking
`collector run`, not an in-process cron library.

## Acceptance criteria

1. **Given** the deployed stack
   **When** 06:00 Africa/Casablanca arrives
   **Then** `collector run` starts automatically.
2. **Given** any run
   **When** it finishes
   **Then** a row in `collector_runs` records start, end, outcome (ok, partial, failed),
   companies attempted, failed, and postings new/updated.
3. **Given** a successful or partial run
   **When** it finishes
   **Then** it pings a healthchecks.io check; if no ping arrives within 26 hours, the
   owner is notified.
4. **Given** a failed run
   **When** it finishes
   **Then** it pings the check's failure URL so the alert fires immediately.

## Out of scope

- The morning digest (M7-02).

## Security notes

The healthchecks ping URL is a secret (env var), never committed or logged.
