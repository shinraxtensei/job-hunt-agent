# M2-08: Tracker polish

- **Status:** Done
- **Milestone:** M2 — Rules and a plain tracker
- **Linear:** [HAM-37](https://linear.app/hamid-corp/issue/HAM-37/m2-08-tracker-polish)
- **Estimate:** ½ day

## Story

As the job seeker scanning the tracker every morning, I want each row to show the place
that made it fit my filters, without repeated words or raw codes, so that I can judge a
lead from one glance at its row.

## Context

Seen on the M2-06 page with real data: "Hybrid +1 · hybrid" (the location is itself a
workplace word), the posting's *first* location even when another one is why it is shown
("Romania +13" for a job shown because one of its countries is Morocco), raw facet values
in the filters ("ai_ml", "staff_plus", "country:MA"), and a search placeholder cut off
at 375 px.

Only the API knows which location matched: location rules are evaluated there
(`preset.Evaluate`, ADR-0012) and the page never sees the rules. So the API adds one
field, `matched_location` (additive; recorded in ADR-0012's consequences). The display
labels are the page's concern and live in one map in `web/`; raw values stay in the URL
and the API.

## Acceptance criteria

1. **Given** a lead whose shown location is itself a workplace word ("Remote",
   "Hybrid", "On-site", any case), or names it ("Remote - EMEA" for a remote job)
   **When** the row is rendered
   **Then** the workplace label isn't appended again: "Hybrid +1 · hybrid" → "Hybrid +1".
   (Table test.)
2. **Given** a lead shown because one of its locations matched the active location rules
   **When** `GET /api/leads` returns it
   **Then** it carries `matched_location`: the first location string, of the first
   member that matched, for which the matching rule holds on its own; the row shows it
   first ("Morocco +13"). When no location rule applied (no preset rules, hidden leads)
   the field is empty and the row falls back to the first location. (Go table test +
   golden files; web table test.)
3. **Given** facet values
   **When** they are displayed (filters, chips, details, hidden reasons)
   **Then** one label map turns them into words: "AI/ML", "Full-stack", "Backend",
   "EMEA", "On-site", "Staff+", country codes into country names, hidden reasons into
   "Role: Sales", "Needs 7+ years". Raw values stay in the URL and the API. (Table test.)
4. **Given** a 375 px screen
   **When** the page opens
   **Then** the search placeholder fits its field ("Search"). (Playwright at 375.)
5. **Given** the changes
   **When** `make check` runs
   **Then** axe still reports 0 violations; the PR shows before/after screenshots.

## Out of scope

- Text search in the API, new filters, per-lead actions (M6).

## Security notes

`matched_location` is one of the posting's own location strings: untrusted text,
rendered as text only, like the rest.
