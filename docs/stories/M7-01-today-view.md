# M7-01: Today view

- **Status:** Done
- **Milestone:** M7 — Today view and more sources
- **Linear:** [HAM-65](https://linear.app/hamid-corp/issue/HAM-65/m7-01-today-view)
- **Estimate:** 1 day

## Story

As the job seeker, I want the tracker to open on what changed since my last visit (new
leads, follow-ups due, postings that closed), so that a quick look after `make collect`
tells me what to do today.

## Context

Replaces the M7 digest (moved to D on 2026-11-09: no pushed digest while collection is on
demand on the Mac). It reads what M2 to M6 already store: leads (`first_seen_at`, fit),
follow-ups (M6-04), closed postings (M2-04).

## Owner input

**OI-20**: what "since my last visit" means (default below).

## Acceptance criteria

1. **Given** a previous visit
   **When** I open the Today view (the tracker's default view; `?view=today`)
   **Then** I see three sections:
   - **New since <time>:** leads first seen after the previous visit and shown by the
     default preset, best fit first, with fit, company, title and a link to the lead;
   - **Follow-ups due:** M6-04's list;
   - **Closed since <time>:** postings closed after the previous visit for leads I acted
     on (any status but `new` or `skipped`), so I know an application went quiet.
   Each section says "Nothing new" when empty.
2. **Given** OI-20's default
   **When** the view is opened
   **Then** the previous visit is the start of the last visit that was at least 30
   minutes ago, stored in Postgres (one owner). A reload doesn't empty the list. "Mark
   all seen" starts a new visit now.
3. **Given** no visit yet
   **When** the view is opened
   **Then** "new" means the last 24 hours.
4. **Given** the page
   **When** Playwright runs at 375 and 1280 px, light and dark
   **Then** axe finds 0 violations, there's no horizontal scroll, and posting text is
   rendered as text only.

## Results (2026-11-09)

- **Data:** a `visits` table holds start, last seen and "marked seen".
  - `POST /api/visits` records an opening of Today: a new visit after 30 minutes away,
    else the current one goes on.
  - `POST /api/visits/seen` is "Mark all seen".
  - `GET /api/today` returns "since", the new leads the default preset shows (best
    first, at most 50; the count is complete) and the closed postings of leads you
    acted on. All writes go through the ADR-0022 guards.
- **The page:** Today (M7-00 design) records the visit first, then shows "Since
  <weekday time> · Mark all seen", the tiles and the three sections; closed cards say
  when they closed.
- **Bug found by the test:** when a lead's posting closes, its group's canonical posting
  changes (ADR-0009), so closed leads are now matched by group, not posting id.
- **Tests:** `TestSince` (table), `TestToday` (visit lifecycle, closed, mark seen), the
  golden `today_first_visit`, and Playwright "Today shows tiles and sections". Each check
  was broken on purpose and its test seen to fail.

## Out of scope

- Push (email, Telegram): D. Per-device visits.

## Security notes

- Read-only, except recording a visit: an ADR-0022 write.
- Company and title come from postings (untrusted): text only.
