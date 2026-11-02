# 0012. The leads API loads groups and evaluates presets in Go, with a keyset cursor

- **Status:** Accepted
- **Date:** 2026-11-02
- **Story:** docs/stories/M2-05-leads-api.md

## Context

M2-05 adds the first HTTP server. ADR-0011 said the API would build its SQL query from a
preset; but `preset.Evaluate` already holds the semantics (group-level location, "any
scope accepted", hide reasons), and a second implementation in SQL could drift from it and
from `collector preview`. The data is small: ~4,500 groups, ~6,000 jobs (2026-11-02).
Cursor pagination must return every group once even when new jobs arrive between pages.

## Decision

- **Per request, load every group of an active company** (two sqlc queries: groups with
  their canonical job, and every job's places) and evaluate the effective preset in Go
  with `preset.Evaluate`. One code path for `/api/leads`, `/api/leads/hidden`,
  `/api/facets`, `/api/companies` and `collector preview`, so they always agree.
- **Query parameters override the preset per facet**; any location parameter
  (`workplace`, `remote_scope`, `country`, `city`, `track`) replaces the preset's location
  rules with one rule made of those parameters. Values are validated by the same
  `preset.RuleValues` as preset files. `preset=none` starts from no filters.
- **Order and cursor:** newest group first by the group's first-seen time (the earliest
  member's `first_seen_at`, which a new member can't change), then company id, then title
  key. The cursor is that triple, base64url-encoded; the next page is strictly after it.
  New groups sort before any cursor already handed out, so a page never repeats or skips
  a group that existed when paging started.
- A lead's `id` is its canonical posting id; `/api/leads/{id}/members` accepts any member's
  posting id.
- `cmd/api` binds `127.0.0.1:8080` by default (`API_ADDR`), warns on a non-loopback bind,
  and reads presets from `data/presets` on every request (edits apply without a restart).

## Alternatives considered

- **Preset → SQL `WHERE`:** fast at any size, but a second implementation of the filter
  semantics to keep identical to `Evaluate`; not needed at this size.
- **Offset pagination:** repeats or skips rows when new groups arrive.
- **Materialised preset results:** stale after every preset edit (ADR-0011 rejected it).

## Consequences

- **Good:** the API, the preview and the hidden-reasons audit can't disagree; one place
  to change filter semantics.
- **Bad:** every request reads all groups (a few ms now); the ceiling is roughly 50k
  groups. Facet counts evaluate the groups once per facet (five passes), each with that
  facet's own filter removed, so a count says what choosing the value would add.
- **Added by M2-08 (2026-11-02):** each shown lead carries `matched_location`, the
  location that passed the location rules (`preset.MatchedLocation`), so the page can show
  why a lead fits without knowing the rules. Additive; empty when no location rule
  applied.
- **Revisit when:** a request takes over ~200 ms locally, or groups exceed ~50k: move the
  facet filters into SQL and keep `Evaluate` for the location rules and reasons.
