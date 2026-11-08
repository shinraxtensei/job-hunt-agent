# M6-05: Skip reasons

- **Status:** Ready
- **Milestone:** M6 — Full tracker
- **Linear:** [HAM-63](https://linear.app/hamid-corp/issue/HAM-63/m6-05-skip-reasons)
- **Estimate:** ½ day

## Story

As the job seeker, I want to skip a lead in one click and optionally say why, so that it
leaves my list and the reasons can later teach the scorer (M8).

## Context

PRD: "Skip reasons feed scoring" is M8; M6 stores them. A skip is the `skipped` status
(M6-01) plus a reason.

## Acceptance criteria

1. **Given** a lead
   **When** I press Skip
   **Then** its status becomes `skipped` (M6-01), and it leaves the default view (a
   "Skipped" filter shows it again). Undo is offered for a few seconds.
2. **Given** a skip
   **When** I pick a reason (OI-16: the default list) or write one
   **Then** it is stored with the skip, with the lead's score and factor breakdown at
   that moment, for M8.
3. **Given** skips
   **When** `GET /api/skips` is called
   **Then** it returns them with their reasons and scores (the M8 input), newest first.

## Out of scope

- Changing scores from skips (M8).

## Security notes

- Free-text reasons are the owner's: text only, never sent to a model in M6.
