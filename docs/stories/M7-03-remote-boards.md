# M7-03: Remote boards: Remotive and RemoteOK

- **Status:** Done
- **Milestone:** M7 — Today view and more sources
- **Linear:** [HAM-67](https://linear.app/hamid-corp/issue/HAM-67/m7-03-remote-boards-remotive-and-remoteok)
- **Estimate:** 1 day

## Story

As the job seeker, I want remote jobs from boards that allow it collected with the rest,
so that remote roles from companies outside the watch list reach me too.

## Context

Step 0 (`docs/research/m7-sources.md`): Remotive and RemoteOK publish public APIs whose
notices allow use with a link back and the board named as the source.
- Remotive also asks for at most ~4 fetches a day and no resubmission to other sites.
- We Work Remotely, Himalayas and Working Nomads have no readable terms yet (OI-19).

These are boards, not company feeds: one fetch returns many companies. ADR-0005's
"companies" model gets a board-wide source.

## Acceptance criteria

1. **Given** the Remotive and RemoteOK adapters
   **When** `make collect` runs
   **Then** each board is fetched at most once per run, and Remotive at most 4 times a
   day (the adapter refuses more). Only the software-development categories are kept.
   Each posting is stored with its board as the source and its company as the company
   (created board-less, like manual adds, ADR-0015). Same tests as every adapter.
2. **Given** a board posting
   **When** it is shown in the tracker
   **Then** it links to the board's posting URL and names the board ("via Remotive"),
   as their terms ask.
3. **Given** the same job on a board and on a company's own feed
   **When** groups are built
   **Then** they group together when company and title match (ADR-0009), and the
   company's own posting is canonical.
4. **Given** the default preset
   **When** board postings arrive
   **Then** they are classified and filtered like any other (remote scope, seniority,
   role family). Nothing is shown that the preset would hide.

## Results

**Slice 1: Remotive and the board-wide plumbing (2026-11-09).** ADR-0029.
- **Adapter** `internal/source/remotive`: one GET of `remotive.com/api/remote-jobs`. The
  API ignores its `category` parameter (checked live), so the adapter keeps "Software
  Development" and "DevOps / Sysadmin" itself. A job without id, title, company or a
  `https://remotive.com/` URL is skipped and counted. Golden file (2026-11-09, 4 jobs),
  empty, malformed, not-a-list, HTTP error, cancellation and fuzz tests.
- **At most 4 fetches a day:** `MinInterval()` 6 h; the collector records
  `companies.fetched_at` and skips a board fetched more recently (logged, not a failure).
- **Companies:** each job is stored under the company of that name, a watch-list row
  first, else board-less (`FindCompanyByName`); origin `board`. Closing is by source.
- **Groups:** a board posting groups with the company's own feed (same company row and
  title key); the company's posting is canonical (`job_groups` orders board postings last).
- **Shown and attributed:** board groups are listed like manual ones (unless the company is
  excluded) and filtered by the preset; the leads API returns `source`; rows say "via
  Remotive" and the title links to the Remotive job.
- **Live:** 5 software jobs of 4 companies; the preset shows 3 (Fluence Full-Stack
  Developer scored Strong) and hides 2 for seniority, role and location reasons. A second
  `make collect` a minute later didn't fetch Remotive.

**Slice 2: Remote OK (2026-11-09).**
- **Adapter** `internal/source/remoteok`: one GET of `remoteok.com/api`, a JSON array whose
  first element is the legal notice (required: without it the response isn't the job
  list, and an empty result would close every Remote OK posting). Remote OK has no
  categories and tags loosely ("dev" on an accountant), so the adapter keeps jobs tagged
  `dev` or `engineer` and the preset's role filter does the rest. Same six test types;
  6-hour interval as Remotive (Remote OK states none). Its logo is never loaded.
- **Repaired names:** Remote OK sends HTML entities ("&amp;") and UTF-8 read as Latin-1
  ("Sothebyâs"); `job.FixText` undoes both for titles, companies and locations, and
  leaves correct accented text alone.
- **Live:** 65 jobs tagged dev or engineer; the preset shows 22 and hides 43 by role,
  seniority, experience or location. 20 have an unknown role family (property manager,
  tax manager); they are shown, as unknown roles are everywhere, and scored low.
- **Attribution:** "via Remote OK", linked to the Remote OK job. The tracker opens links
  with `rel="noopener noreferrer"` (the owner's privacy; a private tracker, not a site, so
  "follow" links don't apply).

## Out of scope

- We Work Remotely, Himalayas, Working Nomads until their terms are read (OI-19).
- Republishing anything (Remotive's terms).

## Security notes

- Public APIs, no login, no keys; a descriptive User-Agent.
- Board content is untrusted: parsed with size caps and fuzzed, and shown as text.
