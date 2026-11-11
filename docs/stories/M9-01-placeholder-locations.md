# M9-01: Placeholder locations are not places

- **Status:** Done
- **Milestone:** M9 — Cleanup
- **Linear:** [HAM-73](https://linear.app/hamid-corp/issue/HAM-73/m9-01-placeholder-locations-are-not-places)
- **Estimate:** ½ day

## Story

As the job seeker, I want "N/A", "TBD" or a bare "Hybrid" in a posting's location to be
dropped, so that leads show real places, and location rules and scores aren't fooled by
words that name no place.

## Context

Boards fill location fields with placeholders. On 2026-11-11 the stored jobs held
"Hybrid" 386 times, "Remote" 40, "N/A" 21 and "na" 2 as locations: 449 jobs in all. A
work mode on its own is a workplace, which each normaliser already reads; as a location it
only adds noise ("Hybrid +1").

## Acceptance criteria

1. **Given** a posting whose location values include a placeholder ("N/A", "n.a.", "na",
   "-", "TBD", "TBA", "none", "null", or a bare "Remote", "Hybrid", "On-site")
   **When** it is normalised (any board)
   **Then** the placeholder is not a location; real places stay, and a work mode still
   sets the workplace. "Remote - EMEA" stays, and so does "Anywhere" (it means worldwide).
   "NA" in capitals stays: on remote boards it means North America.
2. **Given** the stored jobs
   **When** `collector normalise` runs after the normaliser version bump (3)
   **Then** every job is rewritten and re-classified, and none keeps a placeholder.

## Out of scope

- Classifying more place names (`classify --report-unknown` is for that).

## Security notes

None: a change to how untrusted location text is filtered, pure and table-tested; no new
input, output or dependency.

## Results (2026-11-11)

- `TestNormaliseDropsPlaceholderLocations` across Greenhouse, Lever, Ashby, Personio and
  manual postings; Personio reads its remote flag from the raw office values.
- Local data: 6,809 jobs re-normalised, 0 failed; jobs with a placeholder **449 → 0**.
  The default preset shows 550 leads; nothing newly shown needed extraction.
