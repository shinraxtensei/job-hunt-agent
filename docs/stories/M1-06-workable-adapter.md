# M1-06: Workable adapter

- **Status:** Ready
- **Milestone:** M1 — Collector
- **Linear:** —
- **Estimate:** ½ day

## Story

As the job seeker, I want postings from companies on Workable collected, so that those
companies' openings reach my tracker without me checking their sites.

## Context

Workable's public endpoints are less documented; confirm a supported public endpoint before coding, and stop if only scraping would work. Follow the `source-adapter` skill end to end; it is the definition of done.

## Acceptance criteria

1. **Given** the Workable public API terms
   **When** this story starts
   **Then** the endpoint URL, its terms and robots status are recorded below under
   Security notes before any code is written.
2. **Given** recorded responses in `internal/source/workable/testdata/`
   **When** the golden-file test runs against `httptest`
   **Then** the exact expected `job.Raw` records are produced.
3. **Given** an empty board, a malformed posting, a 404, a 429 with `Retry-After`, and a
   cancelled context
   **When** each is tested
   **Then** the behaviour matches the `source-adapter` skill (empty slice; skip one;
   error naming the company; retry after the delay; prompt stop).
4. **Given** `FuzzParse` seeded with the golden files
   **When** fuzzed for 30 seconds
   **Then** it never panics.
5. **Given** a real company on Workable from the watch list
   **When** `collector run` executes locally
   **Then** its postings are stored and spot-checked by eye against the live site.

## Out of scope

- Normalising fields into the job record (M2-01).

## Security notes

Endpoint, terms and robots status: _fill in at story start._
No login, no bypassing blocks. Raw payloads stored, never logged in full.
