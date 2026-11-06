# M3-08: Score, reason and gaps in the tracker

- **Status:** Done
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

Amended 2026-11-06 from the owner's direction for the story (after M3-06).

1. **Given** `GET /api/leads` or `GET /api/leads/{id}`
   **When** a lead has a score
   **Then** it carries `score` (total with freshness as of now), `fit_level`,
   `not_enough_info`, `breakdown` (each factor's weight, value and points, freshness
   included), `reason`, `gaps`, `cv_variant`, `reason_current`, and `score_status`
   (scored / pending / needs_review / not_queued); score fields are null when not
   scored. Golden files updated.
2. **Given** `sort=score` (the default) or `sort=date`
   **When** I page
   **Then** leads are ordered best first (unscored last) or newest first, with the same
   exactly-once cursor guarantee as M2-05 (integration test).
3. **Given** `fit=<levels>` (and `unscored`)
   **When** I filter
   **Then** only leads at those fit levels are returned, and `/api/facets` counts each
   fit level with the fit filter removed, like the other facets.
4. **Given** the page
   **When** leads are scored
   **Then** each row shows a compact fit-level marker and the score (tabular numbers,
   right-aligned; the level is in text too, never colour alone), best first by default;
   the expanded row shows the reason, the gaps, and the factor breakdown as small bars
   with numbers and a text equivalent for screen readers; "not enough info",
   `needs_review` and "reason from older inputs" are shown plainly. Playwright at 375
   and 1280 px, light and dark: axe 0, no horizontal scroll at 375 px.
5. **Given** a lead whose reason is from older inputs
   **When** I open it
   **Then** the page asks for a fresh reason (`POST /api/leads/{id}/reason`, ADR-0022),
   which only marks it requested; a cross-site request is refused (integration test).
6. **Given** model output is derived from untrusted postings
   **When** it is rendered
   **Then** it is text only (same rules and tests as AC4 of M2-06: an HTML/`javascript:`
   payload in a reason or a gap is inert).

## Out of scope

- Editing or overriding scores (M6/M8). Notifications (M7).

## Security notes

- Model output is untrusted: JSON-encoded by the API, rendered as text by the page.
- The API gets its first write. It only sets `scores.reason_requested_at`, and it
  refuses cross-site requests (ADR-0022), so a page elsewhere can't spend model budget.
