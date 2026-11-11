# M9-02: The location factor scores the member that matched

- **Status:** Done
- **Milestone:** M9 — Cleanup
- **Linear:** [HAM-74](https://linear.app/hamid-corp/issue/HAM-74/m9-02-location-factor-from-the-matched-group-member)
- **Estimate:** ½ day

## Story

As the job seeker, I want a lead's location score to judge the posting that passed my
location rules, so that a role open in "Remote - EMEA" isn't marked down because its
canonical posting is in Austin.

## Context

ADR-0023 stored the matched location's text for the detail, but the factor itself used
the canonical posting's workplace, scopes and countries. ADR-0030 stores which member
matched and scores its place.

## Acceptance criteria

1. **Given** a group whose canonical posting fails the default preset's location rules
   and another member passes
   **When** reconcile runs
   **Then** the canonical job records that member (`matched_posting_id`) next to its
   matched location; the canonical posting is preferred when it passes itself.
2. **Given** a recorded member
   **When** the lead is scored
   **Then** the location factor uses the member's workplace, scopes, countries and
   locations; with none recorded, the job's own.
3. **Given** the change ships
   **When** the worker runs
   **Then** every lead is re-scored once (`SCORE_CODE` 8), with no model call for scores.

## Out of scope

- Re-scoring automatically when only the matched member changes (unchanged rule:
  ADR-0021's inputs).

## Security notes

None: an internal column between Go and Python; no new input or output.

## Results (2026-11-11)

- Tests: `TestMatchNamesTheMember` (preset), `TestReconcileStoresMatchedLocation`
  (extended, integration), `test_job_sql_reads_the_matched_member` (Python contract).
- Local data: 550 shown groups record their member; in 12 it isn't the canonical posting.
  582 of 584 scores re-scored; 5 changed: 4 up (e.g. a Cloudflare role 20.7 → 26.7, its
  location 0.2 → 0.6) and 1 down (a ClickHouse role whose canonical posting is US-only
  remote: the member that passed is in Canada, now scored as such, 48.5 → 44.0).
