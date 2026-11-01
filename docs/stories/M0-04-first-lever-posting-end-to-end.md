# M0-04: First Lever posting end to end

- **Status:** Done
- **Milestone:** M0 — Walking skeleton
- **Linear:** [HAM-18](https://linear.app/hamid-corp/issue/HAM-18/m0-04-first-lever-posting-end-to-end)
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

### Step 0 findings (checked 2026-11-01)

- **Official API:** yes. Lever's public Postings API, documented in
  [lever/postings-api](https://github.com/lever/postings-api) (README, last pushed
  2026-04-23). We use only `GET https://api.lever.co/v0/postings/<site>?mode=json`
  (global instance; EU boards are on `api.eu.lever.co`). Fields used: `id`, `text`,
  `hostedUrl`; the full element is stored as raw JSON.
- **Terms:** the README says all published postings "are publicly viewable" and "may be
  scraped by third parties"; the only documented rate limit is on application POSTs
  (2/s), which we never call. No API key is needed for reads.
- **robots.txt:** `api.lever.co` → `User-agent: * / Allow: / / Crawl-delay: 1`. We make
  one request per run, so the crawl delay is met; M1-03's limiter must keep ≥1s between
  requests. `jobs.lever.co` (the HTML site, which we don't fetch) also publishes
  Cloudflare content signals: `search=yes, ai-train=no`, and nothing either way for
  `ai-input`. We never train on postings; whether scoring with a model counts as
  `ai-input` for the HTML site is moot while we only use the API, but revisit if we
  ever fetch `jobs.lever.co`.
- **Login:** none. **Anti-bot:** none on the JSON API (plain 200 responses, no
  challenge).
- **Our client:** 30s timeout, `User-Agent: job-hunter/0.1
  (+https://github.com/shinraxtensei/job-hunter)`, 32 MiB response cap, errors carry the
  status only (never the body), logs carry counts and posting IDs only.

## Notes for the implementer

- Lever's public postings endpoint is documented in the lever/postings-api repo on
  GitHub; confirm the current URL and JSON shape there rather than guessing.
