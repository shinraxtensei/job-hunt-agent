# M1-05: Ashby adapter

- **Status:** Done
- **Milestone:** M1 — Collector
- **Linear:** [HAM-24](https://linear.app/hamid-corp/issue/HAM-24/m1-05-ashby-adapter)
- **Estimate:** ½ day

## Story

As the job seeker, I want postings from companies on Ashby collected, so that those
companies' openings reach my tracker without me checking their sites.

## Context

Ashby exposes a public posting API per job board name. Follow the `source-adapter` skill end to end; it is the definition of done.

## Acceptance criteria

1. **Given** the Ashby public API terms
   **When** this story starts
   **Then** the endpoint URL, its terms and robots status are recorded below under
   Security notes before any code is written.
2. **Given** recorded responses in `internal/source/ashby/testdata/`
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
5. **Given** a real company on Ashby from the watch list
   **When** `collector run` executes locally
   **Then** its postings are stored and spot-checked by eye against the live site.

## Out of scope

- Normalising fields into the job record (M2-01).

## Security notes

Checked 2026-11-01 (also seen earlier the same day while building the M1-01 watch list):

- **Endpoint:** `GET https://api.ashbyhq.com/posting-api/job-board/<board>?includeCompensation=true`,
  Ashby's documented public job posting API
  ([docs](https://developers.ashbyhq.com/docs/public-job-posting-api)), described as
  returning "all currently published Job Postings" for a job board. No key or auth header.
  Response: `{"apiVersion": "1", "jobs": [...]}`; each job has `title`, `jobUrl`,
  `isListed`, `descriptionHtml`/`descriptionPlain`, `publishedAt`, and (with the flag)
  compensation. Real responses also carry an `id` (UUID, also the last segment of
  `jobUrl`) that the docs don't list; we use it as the source ID and skip jobs without it.
  `isListed: false` jobs "should only be available via direct link", so we leave them out.
- **Terms:** the docs describe the API for building your own careers page and say nothing
  about third-party use, rate limits or caching; nothing prohibits reading published
  postings. Less explicit than Lever ("may be scraped by third parties") or Greenhouse
  ("publicly available"); noted, not a blocker.
- **robots.txt:** `api.ashbyhq.com/robots.txt` answers **401**. Under RFC 9309 §2.3.1.3 a
  4xx robots response means no restrictions. `jobs.ashbyhq.com` (the HTML job board, a
  different host) disallows `/meeting/`, `/b/` and `/api/`; we never request that host.
  No Crawl-delay anywhere, so the skill's default applies: the collector's per-host limiter,
  ≥1s between requests (retries included).
- **Login:** none. **Anti-bot:** none on the JSON API (plain 200s, including one request
  to record the golden file from `linear`).
No login, no bypassing blocks. Raw payloads stored, never logged in full.
