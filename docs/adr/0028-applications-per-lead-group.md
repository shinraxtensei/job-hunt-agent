# 0028. Applications: one per lead group, an append-only status history, a snapshot when applied

- **Status:** Accepted; amended by ADR-0032 (one current application, ended ones set aside) (follows ADR-0009 and ADR-0026; recorded under the owner's working
  mode of 2026-11-08)
- **Date:** 2026-11-08
- **Story:** docs/stories/M6-01-application-status.md

## Context

M6 tracks every application from first action to outcome (PRD statuses). A lead is a
duplicate group (ADR-0009), and its canonical posting can change as postings come and
go. `postings` keeps only the latest payload (ADR-0007), so what was applied to must be
kept separately. M5-04's documents must be linked. M6-03 to M6-05 hang notes, contacts,
reminders and skip reasons on the same record.

## Decision

- **`applications`:** one row per group, keyed by `(company_id, title_key)`. It is
  created when the lead first leaves `new` (OI-12). It holds the current status, the
  first `applied_at`, and a `snapshot` taken when first applied (title, URL, raw payload,
  content hash).
- **`application_events`:** append-only, one row per change, with `auto` for system
  changes (OI-11: approving a draft moves `new`/`shortlisted` to `drafted`). An
  unchanged status records nothing.
- **One SQL statement** (`SetStatus`) creates or updates the application and records the
  event, so the two can't disagree.
- **Documents:** they stay on their posting (ADR-0026). An application lists the sent
  documents of any posting in its group.
- **API:** `POST /api/leads/{id}/status` (ADR-0022 guards) and
  `GET /api/leads/{id}/application`. Leads carry `status`, and can be filtered by it.

## Consequences

- **Good:** the group stays the unit everywhere (leads, drafts, applications), and the
  history and the snapshot survive board changes.
- **Bad:** a company merge (M6-06) must move applications along with jobs.
- **Revisit when:** one group needs two applications (e.g. reapplying a year later).
