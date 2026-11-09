# M8-01: Feedback report and weight tuning

- **Status:** Ready, **scheduled: start on or after 2026-11-30** (after 2–3 weeks of real use)
- **Milestone:** M8 — Feedback and write-up
- **Linear:** [HAM-72](https://linear.app/hamid-corp/issue/HAM-72/m8-01-feedback-report-and-weight-tuning)
- **Estimate:** 1 day

## Story

As the job seeker, I want a monthly report of what I skipped and why, and how my
applications went, set against the score that ranked them, so that the scoring weights
change only when my own decisions show they rank leads wrongly.

## Context

M6-05 stores every skip with its reason and the score as shown at that moment
(`skips.score`: score, fit level, breakdown). M6-01 stores each application's status
history. Scoring weights live in `agents/src/jobhunt_agents/scoring.yaml` (M3-06); a change
re-scores every shown lead with no model call. On 2026-11-09 there were 0 skips and 2
applications: nothing to learn from yet, hence the wait (owner, 2026-11-09).

**Start condition:** at least 30 skips with a reason and 10 applications. Below that, the
report runs but says it hasn't enough data and proposes nothing.

## Acceptance criteria

1. **Given** skips, applications and their scores as shown
   **When** `make feedback` runs
   **Then** it writes a report (git-ignored, under `agents/evals/private/feedback/`):
   - counts by skip reason and by outcome (applied, replied, interviewing, offer,
     rejected);
   - for each skip reason, the mean of the matching factor (location → location,
     seniority → seniority, stack → required + optional, role_type → title) among those
     skips, against the same factor among applied leads;
   - how high skipped and applied leads ranked: the share of skips in the top 20 by score,
     and where the applied leads sat.
2. **Given** the report
   **When** a factor clearly disagrees with the decisions (e.g. many "location" skips
   with full location points)
   **Then** it names the rule or weight that gave those points and proposes a change.
   Each proposal is replayed on the stored leads, with the ranking before and after:
   applied leads should move up and skipped ones down. Nothing is changed automatically.
3. **Given** a proposal the owner accepts
   **When** `scoring.yaml` (or a profile rule) changes in a PR
   **Then** the weights still sum to 100, `make evals-fit SET=dev` doesn't get worse, and
   the held-out set is checked once. The PR links the report's numbers. The decision is
   logged in `docs/feedback/YYYY-MM.md` (no personal content: counts and factor means
   only).
4. **Given** a month later
   **When** the review is due
   **Then** `docs/feedback/README.md` describes the monthly routine (run the report,
   decide, record), and the M8 exit criterion "run once" is met by the first entry.

## Out of scope

- Learning weights automatically, or any model call in the report.
- Skip notes (the owner's words) in the report or any model input: counts and reasons only.
- Changing the honesty guardrails or the tailoring: this is about ranking.

## Security notes

- Skips, notes and outcomes are personal: the report stays git-ignored; committed
  records hold counts and means only.
- No new network access, no new dependency, no model call.

## Notes for the implementer

- The score as shown is in `skips.score`; applications link to their group, whose
  current score is in `lead_scores` (use the score at `applied_at` if stored, else now).
- Replay with `score.py`'s coded factors on the stored extractions: the same path
  `make evals-fit` uses.
