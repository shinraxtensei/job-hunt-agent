# M3-08: Score, reason and gaps in the tracker

- **Status:** Ready
- **Milestone:** M3 — Extraction and scoring
- **Linear:** [HAM-47](https://linear.app/hamid-corp/issue/HAM-47/m3-08-score-reason-and-gaps-in-the-tracker)
- **Estimate:** 1 day

## Story

As the job seeker, I want to see each lead's score, fit level, reason and gaps in the
tracker, and sort by score, so that the best leads are the first I read every morning.

## Context

Builds on the leads API (M2-05, ADR-0012) and the page (M2-06/M2-08). Scores come from
M3-06. A lead may have no score yet (queued, failed, or hidden by the default preset).

## Acceptance criteria

1. **Given** `GET /api/leads`
   **When** a lead has a score
   **Then** it carries `score`, `fit_level`, `reason`, `gaps` and `cv_variant` (null
   when not scored, with `score_status`: pending / needs_review / scored); golden files
   updated.
2. **Given** `sort=score`
   **When** I page
   **Then** leads are ordered by score (unscored last) with the same exactly-once cursor
   guarantee as M2-05 (integration test).
3. **Given** the page
   **When** a lead is scored
   **Then** the row shows the score (tabular, right-aligned) and fit level; the details
   show the reason and gaps; a lead in `needs_review` says so, and a reason generated
   from older inputs (M3-06 AC5) is marked as such until it is regenerated; opening the
   details requests the regeneration. Axe stays at 0, and the
   375 px layout keeps no horizontal scroll.
4. **Given** model output is derived from untrusted postings
   **When** it is rendered
   **Then** it is text only (same rules and tests as AC4 of M2-06: an HTML/`javascript:`
   payload in a reason is inert).

## Out of scope

- Editing or overriding scores (M6/M8). Notifications (M7).

## Security notes

Model output is untrusted: JSON-encoded by the API, rendered as text by the page.
