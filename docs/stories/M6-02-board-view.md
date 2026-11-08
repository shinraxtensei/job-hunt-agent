# M6-02: Board view

- **Status:** Ready
- **Milestone:** M6 — Full tracker
- **Linear:** [HAM-60](https://linear.app/hamid-corp/issue/HAM-60/m6-02-board-view)
- **Estimate:** 1 day

## Story

As the job seeker, I want a board of my applications by status, so that I see at a
glance what is waiting on me and what is waiting on employers.

## Context

PRD: "board view" (it may slip behind the cut line). It reads M6-01's applications.
Same page, design rules and vendored-skills review as M3-08 and M4-05.

## Acceptance criteria

1. **Given** applications
   **When** I open the Board tab
   **Then** I see one column per status (OI-13: the default column set), each card
   with company, title, fit level, days in this status and the next step (a follow-up
   due, M6-04).
2. **Given** a card
   **When** I move it to another column (drag, or a "Move to" menu for keyboard and
   touch)
   **Then** its status changes through M6-01's endpoint, and the history records it.
3. **Given** a phone (375 px)
   **When** the board is shown
   **Then** it is a list grouped by status, with no horizontal page scroll.
4. **Given** the page
   **When** Playwright runs at 375 and 1280 px, light and dark
   **Then** axe finds 0 violations, and moving a card works with the keyboard only.

## Out of scope

- Custom columns. Bulk moves.

## Security notes

- Text only (company and title come from postings, untrusted). The move is an
  ADR-0022 write.
