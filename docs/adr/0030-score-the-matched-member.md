# 0030. The location factor scores the group member that passed the preset

- **Status:** Accepted
- **Date:** 2026-11-11
- **Story:** docs/stories/M9-02-location-from-matched-member.md
- **Amends:** ADR-0023

## Context

A lead is a group of postings for the same role (ADR-0009). The default preset shows the
group when any member passes its location rules, but the scorer read the place of the
canonical posting (ADR-0023 stored only the matched location's text). When the canonical
posting is "Austin, TX" and the member that passed is "Remote - EMEA", the location factor
scored Austin: low points for the very place that made the lead worth showing.

## Decision

- **`jobs.matched_posting_id`** (nullable, references `postings`) is stored next to
  `matched_location` on the canonical job: the member whose place passed. The reconcile
  step writes both (`SetMatchedLocations`); `preset.Match` returns both.
- **The canonical posting is tried first**, so it is the matched member whenever it
  passes; otherwise the first member that passes.
- **The scorer reads that member's place** (workplace, remote scopes, countries,
  locations), falling back to the job's own when nothing matched (`score.JOB_SQL`,
  `rescore._STALE`). Everything else (title, seniority, years, extraction) stays the
  canonical posting's.
- `SCORE_CODE` 8 re-scores every lead once (no model call; a reason is re-written only
  where the breakdown changed).

## Consequences

- The Go ↔ Python contract grows by one column, pinned by
  `TestReconcileStoresMatchedLocation` (Go) and `test_job_sql_reads_the_matched_member`
  (Python).
- A preset change that picks a different member re-scores only when another input
  changes (the existing rescore rule); `SCORE_CODE` or a profile change forces it.
