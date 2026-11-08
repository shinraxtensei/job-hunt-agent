# M6-01: Application status and history

- **Status:** Done
- **Milestone:** M6 — Full tracker
- **Linear:** [HAM-59](https://linear.app/hamid-corp/issue/HAM-59/m6-01-application-status-and-history)
- **Estimate:** 1 day

## Story

As the job seeker, I want every lead I act on to have a status with a dated history, a
snapshot of the posting as I applied to it, and the documents I sent, so that I always
know where each application stands and what the employer received.

## Context

PRD: "Every application has a status and a history, from found to outcome"; statuses
new, shortlisted, drafted, applied, replied, interviewing, offer, rejected, closed,
skipped. `postings` keeps only the latest payload (ADR-0007), so an application keeps
its own snapshot. M5-04's documents are linked here. A new table shared by Go (writes)
and the UI: ADR.

## Owner input

**OI-11** (statuses that change on their own) and **OI-12** (when an application
exists): `docs/stories/M6-owner-input.md`.

## Acceptance criteria

1. **Given** a lead
   **When** I set its status (`POST /api/leads/{id}/status`, ADR-0022 guards)
   **Then** an `applications` row exists for the lead (one per duplicate group), the
   new status is stored with its time, and the old one stays in its history. Any status
   may follow any other; nothing is deleted.
2. **Given** a status change to `applied`
   **When** it is stored
   **Then** the application keeps a snapshot of the posting: title, URL, raw payload and
   `content_hash` at that moment. A later change on the board doesn't alter it.
3. **Given** documents marked sent (M5-04) for the lead
   **When** the application is shown
   **Then** they are listed with it (linked by id), with their sent time.
4. **Given** OI-11's defaults
   **When** a draft is approved
   **Then** a lead still at `new` or `shortlisted` becomes `drafted`, recorded in the
   history as automatic.
5. **Given** the tracker
   **When** I open a lead
   **Then** I see its status (changeable), its history (newest first, with dates), the
   snapshot when applied, and the sent documents. The leads API can filter by status.

## Results (2026-11-08)

- ADR-0028.
- Tests: `TestApplicationStatus`, `TestApproveSetsDrafted`, the goldens (`status` on
  leads), and Playwright "a lead's status and history".
- Each check was broken on purpose and its test seen to fail. The first run let an
  overwritten `applied_at` through; the test now checks it keeps the first time.

## Out of scope

- The board (M6-02). Notes and contacts (M6-03). Reminders (M6-04). Skip reasons
  (M6-05).

## Security notes

- The snapshot holds posting text (untrusted): stored as JSON, rendered as text only,
  never logged.
- Writes go through the ADR-0022 guards.
