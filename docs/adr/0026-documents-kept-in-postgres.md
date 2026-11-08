# 0026. Exported documents: immutable rows in Postgres, sent marked by the owner

- **Status:** Accepted (follows ADR-0025; recorded under the owner's working mode of
  2026-11-08)
- **Date:** 2026-11-08
- **Story:** docs/stories/M5-04-document-versions.md

## Context

The PRD says "every sent version is kept". M5-03 exports an approved draft as two PDFs
(the worker, in Python, writes them); the tracker (Go) lists them and the owner marks
what they sent. M6 will link a sent document to its application. A background check
compares what the employer received with the truth, so the stored bytes must be the
ones sent, unchanged.

## Decision

- A `documents` table: one row per PDF, with the draft, the lead, the kind (`cv` or
  `letter`), the bytes, their SHA-256, the evidence version and the renderer version.
- Rows are never changed or deleted. A trigger refuses both, except setting `sent_at`
  once. It applies to every role, so neither the worker nor the API can bypass it.
- `POST /api/documents/{id}/sent` (ADR-0022 guards) sets `sent_at`. Marking a document
  again returns the first time. The system sends nothing.
- The bytes live in Postgres, so `make backup` and the smoke test's restore cover them.

## Alternatives considered

- **Files on disk next to the database:** a second thing to back up and keep in step;
  nothing stops an edit.
- **Re-render on download:** the bytes would change with the renderer or fonts, so
  "what was sent" would be lost.

## Consequences

- **Good:** what was sent is provable (bytes and hash); one backup holds everything.
- **Bad:** the database grows by about 100 KB per export; fine at a few a day.
- **Revisit when:** documents need deleting (a data-protection request); that would take
  a migration that drops the trigger for one statement.
