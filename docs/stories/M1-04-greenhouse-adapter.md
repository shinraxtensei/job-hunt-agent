# M1-04: Greenhouse adapter

- **Status:** Done
- **Milestone:** M1 — Collector
- **Linear:** [HAM-23](https://linear.app/hamid-corp/issue/HAM-23/m1-04-greenhouse-adapter)
- **Estimate:** ½ day

## Story

As the job seeker, I want postings from companies on Greenhouse collected, so that those
companies' openings reach my tracker without me checking their sites.

## Context

Greenhouse exposes a public Job Board API per board token. Follow the `source-adapter` skill end to end; it is the definition of done.

## Acceptance criteria

1. **Given** the Greenhouse public API terms
   **When** this story starts
   **Then** the endpoint URL, its terms and robots status are recorded below under
   Security notes before any code is written.
2. **Given** recorded responses in `internal/source/greenhouse/testdata/`
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
5. **Given** a real company on Greenhouse from the watch list
   **When** `collector run` executes locally
   **Then** its postings are stored and spot-checked by eye against the live site.

## Out of scope

- Normalising fields into the job record (M2-01).

## Security notes

Checked 2026-11-01 (also seen earlier the same day while building the M1-01 watch list):

- **Endpoint:** `GET https://boards-api.greenhouse.io/v1/boards/<board_token>/jobs?content=true`,
  Greenhouse's official Job Board API ([docs](https://docs.greenhouse.io/job-board.html)).
  The response is `{"jobs": [...], "meta": {"total": N}}`; each job has a numeric `id`,
  `title`, `absolute_url`, `updated_at`, `location`; `content=true` adds the HTML
  description, departments and offices, which we store raw so extraction can be rerun
  without refetching. One request returns the whole board.
- **Terms:** the docs say "Job Board data is publicly available, so authentication is not
  required for any GET endpoints." Only application POSTs need a key; we never send one.
  No rate limit, caching rule or acceptable-use clause is documented for GETs.
- **robots.txt** (`boards-api.greenhouse.io`): `User-agent: * / Disallow: /embed/`. We use
  only `/v1/boards/`. No Crawl-delay is given, so the skill's default applies: the
  collector's per-host limiter, ≥1s between requests (retries included).
- **Login:** none. **Anti-bot:** none on the JSON API (plain 200s during M1-01's checks).
No login, no bypassing blocks. Raw payloads stored, never logged in full.
