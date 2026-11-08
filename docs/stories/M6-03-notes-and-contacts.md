# M6-03: Notes and contacts

- **Status:** Done
- **Milestone:** M6 — Full tracker
- **Linear:** [HAM-61](https://linear.app/hamid-corp/issue/HAM-61/m6-03-notes-and-contacts)
- **Estimate:** 1 day

## Story

As the job seeker, I want dated notes and the people I deal with on each application,
so that I remember what was said and who to follow up with.

## Context

PRD: "notes, contacts". Contacts are other people's personal data (OI-14).

## Acceptance criteria

1. **Given** an application
   **When** I add a note
   **Then** it is stored with its time and shown newest first. A note can be edited
   (its edit time shown) or deleted, with a confirmation.
2. **Given** an application
   **When** I add a contact (name, role, email, LinkedIn URL, phone; only the name is
   required)
   **Then** it is stored with the application and shown. Its email and URL are
   validated (format only) and shown as text, not as links that open on their own.
3. **Given** notes and contacts
   **When** anything is logged, sent to a model or exported
   **Then** neither is included: they never leave the database (test on the model
   client's requests and the logs).

## Results (2026-11-08)

- `notes` and `contacts` tables. Notes can be added, edited (the edit time is kept) and
  deleted with a confirmation. Contact fields are checked for format only: email,
  `https://` LinkedIn hosts only, a phone number. Addresses are shown as text, not links.
- **Decision:** a note or contact on a lead still at `new` creates its application at
  `new`, with no status event. ADR-0028 created it only when the lead left `new`; this
  just adds a place to hang the note.
- AC3: a Go test forces a database error on a request carrying a planted note, then
  checks the log has the failure but not the note or the contact. A Python test checks no
  agent code reads `notes` or `contacts`, so neither can reach a model.
- Each check was broken on purpose and its test seen to fail.

## Out of scope

- Importing contacts. Sending email.

## Security notes

- Third parties' personal data: stored only in Postgres and in backups; no model, no
  logs (OI-14).
- Notes are free text from the owner: rendered as text only.
