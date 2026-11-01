# M0-04: First Lever posting end to end

- **Status:** Ready
- **Milestone:** M0 — Walking skeleton
- **Linear:** —
- **Estimate:** 1 day

## Story

As the job seeker, I want the collector to fetch real postings from one company's Lever
board and store them, so that the whole path from the web to the database works before
anything else is built.

## Context

The tracer bullet. Deliberately minimal: one company from an env var, no scheduler, no
concurrency. M1-03 completes this adapter. Follow the `source-adapter` skill.

## Acceptance criteria

1. **Given** `COLLECTOR_LEVER_COMPANY` set to a company with a public Lever board
   **When** I run `go run ./cmd/collector once`
   **Then** its open postings are inserted into `postings` with the raw JSON stored, and
   the command logs (slog, JSON) how many were new and how many already existed.
2. **Given** the same command run twice
   **When** the second run finishes
   **Then** no duplicate rows exist and `last_checked_at` is updated.
3. **Given** a recorded Lever response in `internal/source/lever/testdata/`
   **When** the parse test runs against an `httptest` server
   **Then** the produced records match the golden expectation exactly.
4. **Given** the Lever endpoint returns 500
   **When** the collector runs
   **Then** it exits non-zero with an error naming the company, and nothing partial is
   written.
5. **Given** any run
   **When** I read the logs
   **Then** they contain counts and IDs, never full posting text.

## Out of scope

- Rate limiting, retries, fuzzing, multiple companies (all M1).

## Security notes

Check Lever's public postings API terms and record them in this file before coding
(step 0 of the `source-adapter` skill). Posting content is untrusted: stored as data,
never logged in full. HTTP client has a timeout.

## Notes for the implementer

- Lever's public postings endpoint is documented in the lever/postings-api repo on
  GitHub; confirm the current URL and JSON shape there rather than guessing.
