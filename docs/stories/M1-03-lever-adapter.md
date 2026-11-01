# M1-03: Lever adapter

- **Status:** Done
- **Milestone:** M1 — Collector
- **Linear:** [HAM-22](https://linear.app/hamid-corp/issue/HAM-22/m1-03-lever-adapter)
- **Estimate:** ½ day

## Story

As the job seeker, I want postings from companies on Lever collected, so that those
companies' openings reach my tracker without me checking their sites.

## Context

M0-04 started this adapter; this story completes it to the skill's standard. Follow the `source-adapter` skill end to end; it is the definition of done.

## Acceptance criteria

1. **Given** the Lever public API terms
   **When** this story starts
   **Then** the endpoint URL, its terms and robots status are recorded below under
   Security notes before any code is written.
2. **Given** recorded responses in `internal/source/lever/testdata/`
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
5. **Given** a real company on Lever from the watch list
   **When** `collector run` executes locally
   **Then** its postings are stored and spot-checked by eye against the live site.
6. **Given** a board where one posting contains a NUL character (`\u0000`) in any field
   **When** the company is collected
   **Then** that posting is skipped and counted as skipped, and the rest of the company's
   postings are still stored. (Postgres rejects NUL in `text` and `jsonb`; today one such
   posting fails the whole board. Found in M0-04.)

## Out of scope

- Normalising fields into the job record (M2-01).

## Security notes

Re-verified 2026-11-01 (same day as M0-04's step 0, recorded in
`docs/stories/M0-04-first-lever-posting-end-to-end.md`); nothing changed:

- **Endpoint:** `GET https://api.lever.co/v0/postings/<site>?mode=json`, the official public
  Postings API documented in [lever/postings-api](https://github.com/lever/postings-api)
  (repo last pushed 2026-04-23, not archived). No key for reads. One request returns the
  whole board (`skip`/`limit` exist; not needed: the largest board seen, 301 postings, came
  back in one response).
- **Terms:** published postings "are publicly viewable" and "may be scraped by third
  parties". The only documented rate limit is on application POSTs, which we never send.
- **robots.txt** (`api.lever.co`): `User-agent: * / Allow: / / Crawl-delay: 1`. Every
  request, retries included, waits on the collector's per-host limiter (≥1s).
- **Login:** none. **Anti-bot:** none on the JSON API.
No login, no bypassing blocks. Raw payloads stored, never logged in full.
