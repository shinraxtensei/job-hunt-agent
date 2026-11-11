# 0032. One current application per lead group; ended ones can be set aside

- **Status:** Accepted
- **Date:** 2026-11-11
- **Story:** docs/stories/M9-05-reapply.md
- **Amends:** ADR-0028

## Context

ADR-0028 keyed applications by group: one row per `(company_id, title_key)`, ever. When a
company reposts a role you were rejected from months ago, there was no way to apply again
without overwriting the first application's history.

## Decision

- **`applications.archived_at`:** an application that ended (status rejected or closed)
  can be set aside. The unique key becomes a partial index: one **current** application
  per group (`archived_at IS NULL`), any number set aside.
- **`POST /api/leads/{id}/reapply`** (ADR-0022 guards) sets the current application aside
  when it is rejected or closed; otherwise 409. The lead is then `new`, and its next status
  creates a new application through `SetStatus` as usual.
- **Every query that means "the lead's application" reads the current one** (leads,
  board, follow-ups, notes, skips, Today's closed list, merge conflicts).
- **`GET /api/leads/{id}/application`** adds `previous`: the set-aside applications, newest
  first. An application lists the documents sent after the previous one was set aside;
  earlier ones stay with their application.

## Consequences

- Nothing is deleted: set-aside applications keep their history, notes, contacts, skips
  and documents.
- Skipped applications aren't set aside: a skip is undone by changing the status.
