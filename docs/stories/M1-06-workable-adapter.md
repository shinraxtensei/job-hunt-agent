# M1-06: Workable adapter

- **Status:** Backlog
- **Milestone:** M7 — Digest and more sources (moved from M1 on 2026-11-01; see the roadmap change log)
- **Linear:** [HAM-25](https://linear.app/hamid-corp/issue/HAM-25/m1-06-workable-adapter)
- **Estimate:** ½ day

## Story

As the job seeker, I want postings from companies on Workable collected, so that those
companies' openings reach my tracker without me checking their sites.

## Context

Workable's public endpoints are less documented; confirm a supported public endpoint before coding, and stop if only scraping would work.

**Found in M1-01 (2026-11-01):** Workable's help centre lists
`www.workable.com/api/accounts/{subdomain}` as a public endpoint; it redirects (302) to
`apply.workable.com/api/v1/widget/accounts/{subdomain}`. That widget endpoint answered
200 with `"jobs": []` for **every** account tried (Vinted, Chari, Glovo, Qdrant, Fly.io,
Yassir, ...) and 404 for unknown ones. Vinted certainly has open roles, so either the
widget no longer lists jobs or these companies don't post through Workable.
**First task:** find the correct public jobs endpoint (official docs or Workable
support), using Vinted as the test case: it must return Vinted's current openings. If no
documented public endpoint lists jobs, stop and say so (no scraping). Until then, Workable
companies stay out of `data/companies.yaml` and are listed in the roadmap's M7 backlog;
the one exception is Yassir, which is in the file as `excluded: true` and never collected. Follow the `source-adapter` skill end to end; it is the definition of done.

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
