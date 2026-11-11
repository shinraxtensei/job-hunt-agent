# M9-05: Reapply after a closed application

- **Status:** Done
- **Milestone:** M9 — Cleanup
- **Linear:** [HAM-77](https://linear.app/hamid-corp/issue/HAM-77/m9-05-reapply-after-a-closed-application)
- **Estimate:** ½ day

## Story

As the job seeker, I want to apply again to a role I was rejected from or that closed, so
that a repost starts a fresh application while the first one's history is kept.

## Acceptance criteria

1. **Given** a lead whose application is rejected or closed
   **When** I click "Apply again"
   **Then** that application is set aside (kept), the lead is new again, and its next
   status creates a new application with its own history.
2. **Given** an active application (any other status)
   **When** reapply is asked
   **Then** it is refused (409) and nothing changes; an unknown lead is 404.
3. **Given** set-aside applications
   **When** the lead's History tab opens
   **Then** each is listed ("Earlier application: Rejected on …, applied …"); the board,
   follow-ups and Today count only the current one.

## Out of scope

- Setting aside a skipped application (undo the skip instead).

## Security notes

A new write endpoint behind the ADR-0022 guards (same-origin JSON only, loopback Host);
nothing is deleted.

## Results (2026-11-11)

- `TestReapply` (refusals, set aside, new application, previous listed, one current row);
  e2e "apply again after a rejection" (the button posts, History lists the earlier one,
  axe clean). Existing application, skip, notes, follow-up, board and Today tests green.
