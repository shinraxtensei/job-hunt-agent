# M1-01: Watch list seed and companies sync

- **Status:** Ready
- **Milestone:** M1 — Collector
- **Linear:** [HAM-20](https://linear.app/hamid-corp/issue/HAM-20/m1-01-watch-list-seed-and-companies-sync)
- **Estimate:** ½ day (plus time choosing the 50 companies)

## Story

As the job seeker, I want my target companies kept in a reviewed file that syncs to the
database, so that the watch list is versioned and easy to change.

## Context

Answers the PRD's first open question. Start from the 86 researched companies in the
September "Departures" tracker and pick 50 that use Lever, Greenhouse, Ashby or Workable.

## Acceptance criteria

1. **Given** `data/companies.yaml` with name, board_type, board_slug, track (morocco,
   remote, relocation, consulting) and optional `excluded: true`
   **When** I run `go run ./cmd/collector companies sync`
   **Then** the `companies` table matches the file: new rows inserted, changed rows
   updated, rows removed from the file marked inactive (never deleted).
2. **Given** a file entry with an unknown board_type or a missing slug
   **When** sync runs
   **Then** it fails before writing anything and lists every invalid line.
3. **Given** the seed file
   **When** it is reviewed
   **Then** it holds at least 50 active companies, and Yassir is present with
   `excluded: true`.
4. **Given** sync is run twice with no file change
   **When** the second run finishes
   **Then** it reports 0 inserted, 0 updated.
5. **Given** the `companies` table
   **When** this story's migration is applied
   **Then** `(board_type, board_slug)` is UNIQUE, and the collector's `companyID`
   (`internal/collector`) is a single upsert instead of select-then-insert, so concurrent
   runs can't create duplicate companies. (Left as a `ponytail:` note in M0-04.)

## Out of scope

- Career-page scraping for companies without an ATS board (M7).

## Security notes

The file holds public company data only.
