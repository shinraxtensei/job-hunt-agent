# M2-05: Leads API (Go)

- **Status:** Ready
- **Milestone:** M2 — Rules and a plain tracker
- **Linear:** [HAM-34](https://linear.app/hamid-corp/issue/HAM-34/m2-05-leads-api-go)
- **Estimate:** 1 day

## Story

As the job seeker, I want a small read-only HTTP API over my leads, so that the tracker
page (M2-06) and, later, the agents have one stable way to read them.

## Context

First HTTP server in the repo (`cmd/api`, `internal/httpapi`, Go 1.22+ `net/http`
routing, no framework, per CLAUDE.md). Read-only in M2; writes arrive with the full
tracker in M6. No auth: local only now, behind Tailscale after Deploy (ADR-0002).

## Acceptance criteria

1. **Given** the API is running
   **When** I call `GET /api/leads`
   **Then** I get canonical job records shown by the `default` preset (M2-03) as JSON,
   newest first, each with: id, company name and track, title, all facets (role family,
   seniority, workplace, remote scope, locations, experience minimum, closed), team,
   published-at, first-seen-at, URL, and the number of duplicates folded into it.
2. **Given** query parameters for any facet (`role_family`, `seniority`, `workplace`,
   `remote_scope`, `country`, `city`, `track`, `company`, `max_experience`, `closed`),
   plus `new` (first seen in the last 24 h), `since` and `preset`
   **When** they are combined
   **Then** results are filtered accordingly. Explicit parameters override the preset's
   value for that facet; `preset=none` starts from no filters. Invalid parameters or an
   unknown preset return 400 with a JSON error naming the parameter.
2b. **Given** `GET /api/leads/hidden?preset=default`
   **When** called
   **Then** it returns the jobs that preset hides, each with the filters that hid it, so
   I can audit my defaults.
2c. **Given** `GET /api/facets` (same filter parameters) and `GET /api/presets`
   **When** called
   **Then** facets returns every facet value with its count under the current filters
   (to build the filter controls), and presets lists the available presets with their
   filters.
3. **Given** many results
   **When** I page
   **Then** cursor pagination (`limit` default 50, max 200, `cursor` opaque) returns every
   record exactly once even when new records arrive between pages (integration test).
4. **Given** `GET /api/companies` and `GET /healthz`
   **When** called
   **Then** companies list name, track, board, active, excluded and counts of open/kept
   leads; healthz returns 200 with the commit SHA and checks the database is reachable.
5. **Given** the server
   **When** it starts
   **Then** it listens on `127.0.0.1` by default, sets read/write/idle timeouts and
   `Content-Type: application/json`, adds `X-Content-Type-Options: nosniff`, and shuts
   down cleanly on SIGINT/SIGTERM (test like M1-02's signal test).
6. **Given** the response shapes
   **When** the API changes
   **Then** golden-file tests of the JSON for each endpoint fail, so the contract can't
   change by accident.

## Out of scope

- Writes (shortlist, skip, status): M6. Auth, per-user presets: milestone S. Serving the
  web app: M2-06.

## Security notes

Responses carry untrusted posting text (titles, locations): JSON-encoded only, never
HTML. SQL only through sqlc. Connects as the app role. Bound to loopback by default; the
bind address is config, and binding to anything else logs a warning.
