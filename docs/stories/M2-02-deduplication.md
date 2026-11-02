# M2-02: Deduplication

- **Status:** Done
- **Milestone:** M2 — Rules and a plain tracker
- **Linear:** [HAM-31](https://linear.app/hamid-corp/issue/HAM-31/m2-02-deduplication)
- **Estimate:** 1 day

## Story

As the job seeker, I want the same role to appear once in my leads even when a company
posts it several times, so that the daily list isn't padded with copies.

## Context

PRD: "No deduplication. The same role posted on three sites appeared as three leads."
Today each company has one board, so cross-board duplicates are rare. The duplicates that
do happen: the same role posted once per city ("Backend Engineer – Berlin", "– Remote"),
and a role closed and re-posted under a new id. Builds on M2-01's job record.

## Acceptance criteria

1. **Given** job records
   **When** they are grouped
   **Then** records with the same company and the same normalised title are one
   duplicate group. Normalised title: lowercase, accents folded, punctuation and
   whitespace collapsed, location and work-mode suffixes removed ("– Remote",
   "(Berlin)", "| Hybrid"); seniority words are kept ("Senior Backend Engineer" and
   "Backend Engineer" are different roles).
2. **Given** a duplicate group
   **When** the canonical record is chosen
   **Then** it is the open record seen first; the group's locations and workplaces are the
   union of its members', so the lead shows "Berlin, Remote" once.
3. **Given** a table of title pairs (at least 25, including French titles and the real
   duplicates found in the local database)
   **When** the normaliser is tested
   **Then** every pair is grouped or kept apart as the table says. Pairs that must stay
   apart include different seniority, different stack ("Backend Engineer, Go" vs
   ", Java") and different team.
4. **Given** the collector stores new or changed postings, or `collector normalise` runs
   **When** it finishes
   **Then** affected groups are recomputed in the same transaction; an unchanged company's
   groups are untouched.
5. **Given** the local database
   **When** grouping has run
   **Then** the PR reports how many records collapsed into how many groups, with five
   random groups listed for a manual check.

## Out of scope

- Cross-company duplicates (agencies re-posting a client's job), fuzzy matching on
  descriptions. Revisit if real duplicates slip through.

## Security notes

None new: works on already-stored data. Titles are untrusted text; normalising must not
use them in regular expressions built at runtime.

## Notes for the implementer

- Pure function `job.TitleKey(title string) string` with a big table test; the grouping
  query is then a plain `GROUP BY company_id, title_key`.
- Store the group as a column on the job record (`group_key`) rather than a separate
  table, unless the ADR from M2-01 says otherwise.
