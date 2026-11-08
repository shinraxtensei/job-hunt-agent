# M6-04: Follow-up reminders

- **Status:** Done
- **Milestone:** M6 — Full tracker
- **Linear:** [HAM-62](https://linear.app/hamid-corp/issue/HAM-62/m6-04-follow-up-reminders)
- **Estimate:** 1 day

## Story

As the job seeker, I want to be reminded 7 days after applying when I've heard nothing,
with a follow-up message ready to copy, so that no application goes quiet by accident.

## Context

PRD: "Follow-up prompt 7 days after applying with no reply". The digest (M7) will carry
reminders by its channel; here they are in the tracker. The message makes no claim about
the owner, so it follows OI-15.

## Acceptance criteria

1. **Given** an application `applied` 7 days ago (OI-15: the delay) with no later
   status
   **When** I open the tracker
   **Then** it shows in "Follow-ups due" (and on its board card), with the days since
   applying.
2. **Given** a reminder
   **When** I open it
   **Then** I see a follow-up message to copy (OI-15: a fixed template filled by code
   with the job title, company and application date; no claims about me), and the
   contact's name when there is one (M6-03).
3. **Given** a reminder
   **When** I mark it done or snooze it (3 or 7 days)
   **Then** it leaves the list until the snooze ends. A status change after `applied`
   also ends it. All of it is in the history.
4. **Given** the message
   **When** it is shown
   **Then** nothing is sent; copying is the owner's action (guardrail rule 6).

## Results (2026-11-08)

- **"Follow-ups due"** sits above the leads. It lists applications at least 7 days after
  `applied` that are still `applied`, not done, and not snoozed past now.
- **The message** is OI-15's template, filled in by Go (`FollowupMessage`):
  - "Hello {name}," or "Hello,";
  - the job title, or "the open role" when the title looks like an instruction (the same
    check as M5-01's letter opening);
  - every value collapsed to one line;
  - your name from the evidence bank.
- **Copy, Done, Snooze 3 or 7 days.** Done and snooze are recorded in the history.
- **Clock:** every history entry uses the database's clock. The first version wrote
  follow-up entries with the API's clock, which put them out of order.
- **Tests:** each check was broken on purpose and its test seen to fail.

## Out of scope

- Sending the message. Reminders for interviews. The digest (M7).

## Security notes

- The template takes the title and company from the posting (untrusted): one line,
  capped, and an instruction-like title is replaced, as in the letter's opening (M5-01).
