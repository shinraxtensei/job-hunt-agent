# M5-04: Document versions

- **Status:** Done
- **Milestone:** M5 — Documents
- **Linear:** [HAM-58](https://linear.app/hamid-corp/issue/HAM-58/m5-04-document-versions)
- **Estimate:** ½ day

## Story

As the job seeker, I want every exported document kept as it was, with what I sent
marked, so that I can always see exactly what an employer received.

## Context

PRD: "every sent version is kept". Linking a document to an application needs M6's
applications, so that link moves to M6 (roadmap change in this PR); here a document is
linked to its draft and lead.

## Acceptance criteria

1. **Given** an export (M5-03)
   **When** it is stored
   **Then** each PDF is one `documents` row: draft, lead, kind (`cv` or `letter`), the
   bytes, their SHA-256, the versions it came from, and when it was made. Rows are
   never updated or deleted (a trigger refuses both, except setting `sent_at` once).
2. **Given** a document
   **When** I mark it sent on the review page (ADR-0022 POST)
   **Then** `sent_at` is set once; marking it again changes nothing. Nothing is sent by
   the system.
3. **Given** a lead
   **When** its drafts are listed
   **Then** each draft shows its documents, newest first, with sent ones marked.
4. **Given** `make backup`
   **When** it runs
   **Then** the documents are in the dump (they live in Postgres; restore test).

## Results (2026-11-08)

- ADR-0026. The trigger refuses every change except setting `sent_at` once, for any role.
- The API lists each draft's documents (no bytes) and adds `POST /api/documents/{id}/sent`.
  The review page shows them with "Mark as sent".
- AC4: the smoke test checks the keep-trigger after a backup is restored. The bytes are
  in the same dump.
- The PDF download (`GET /api/documents/{id}/pdf`) comes with M5-03, which writes the
  rows.
- Each Go check was broken on purpose and its test seen to fail (in the PR).

## Out of scope

- Linking to applications (M6). Deleting documents.

## Security notes

- PDFs are personal data, stored only in Postgres; never logged.
- The mark-sent write goes through the ADR-0022 guards.
