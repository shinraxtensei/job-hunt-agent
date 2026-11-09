# M7-03: Remote boards: Remotive and RemoteOK

- **Status:** Ready
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

## Out of scope

- We Work Remotely, Himalayas, Working Nomads until their terms are read (OI-19).
- Republishing anything (Remotive's terms).

## Security notes

- Public APIs, no login, no keys; a descriptive User-Agent.
- Board content is untrusted: parsed with size caps and fuzzed, and shown as text.
