# M9-04: Board: dropping on Done asks rejected, skipped or closed

- **Status:** Done
- **Milestone:** M9 — Cleanup
- **Linear:** [HAM-76](https://linear.app/hamid-corp/issue/HAM-76/m9-04-board-done-asks-rejected-skipped-or-closed)
- **Estimate:** ½ day

## Story

As the job seeker, I want dropping a card on Done to ask whether I was rejected, skipped
it, or it closed, so that my outcomes are recorded right (M8-01 learns from them).

## Context

Done holds three statuses (OI-13); a drop there set "closed" whatever happened.

## Acceptance criteria

1. **Given** a card dragged onto Done
   **When** it is dropped
   **Then** a dialog asks "Why is it done?" with Rejected, Skipped and Closed; Cancel (or
   Esc) moves nothing.
2. **Given** an answer
   **When** it is chosen
   **Then** Rejected and Closed go through the status endpoint; Skipped goes through the
   skip endpoint, which records the score as shown (M6-05).
3. The card's "Move to" menu is unchanged (it already names each status), and is how a
   phone moves cards (no drag and drop there).

## Out of scope

- A skip reason in the dialog (the lead's Overview asks for one after a skip).

## Security notes

None: the same two same-origin writes as before (ADR-0022).

## Results (2026-11-11)

- e2e "dropping on Done asks rejected, skipped or closed" (1280 px, light and dark, axe
  clean with the dialog open); the existing board test still passes at every width.
