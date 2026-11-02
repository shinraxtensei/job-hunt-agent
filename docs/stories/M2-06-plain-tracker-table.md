# M2-06: Plain tracker table (TypeScript)

- **Status:** In progress
- **Milestone:** M2 — Rules and a plain tracker
- **Linear:** [HAM-35](https://linear.app/hamid-corp/issue/HAM-35/m2-06-plain-tracker-table-typescript)
- **Estimate:** 1½ days (split into scaffold + table if it runs over)

## Story

As the job seeker, I want one page I open each morning that lists my new leads, newest
first, with my profile's filters already applied and every filter one click away, so that
I can decide in minutes what to look at, and widen the search whenever I want.

## Context

First TypeScript in the repo (`web/`). Reads only from the M2-05 API. Must be usable on a
phone (PRD non-goal: no mobile app, but the web UI must work on one). Stack choice is an
ADR (default proposal: Vite + React + TypeScript, no component library; built files
embedded in the Go API binary so the tool stays one process).

## Acceptance criteria

1. **Given** `make up`, the API and `make web-dev`
   **When** I open the page
   **Then** I see a table of leads from `GET /api/leads`: company, title (linking to the
   posting), locations, workplace, published / first-seen date, duplicates count, flags.
2. **Given** the page loads with no filters in the URL
   **When** it renders
   **Then** the `default` preset is applied, and the filter bar shows every facet from
   `GET /api/facets` (role family, seniority, workplace, remote scope, country/city,
   track, company, max experience, open/closed, new) with counts.
2b. **Given** I change any filter, or pick another preset
   **When** the table updates
   **Then** the filters are in the URL (a filtered view can be bookmarked or shared), a
   "Reset to my defaults" control returns to the preset, and a line says how many jobs
   the current filters hide with a link to the hidden view, which shows why each job
   was hidden (`/api/leads/hidden`).
2c. **Given** a lead with more than one member in its duplicate group
   **When** it is shown
   **Then** it reads "N postings" and expands to the list from
   `GET /api/leads/{id}/members` (title, team, department, locations, workplace, first
   seen, link), so every merge can be checked by eye; a lead with one posting shows no
   count.
3. **Given** a phone-width screen (375 px)
   **When** I open the page
   **Then** it is readable without horizontal page scroll (rows become cards); checked
   with a Playwright test at 375 px and 1280 px.
4. **Given** posting text and URLs are untrusted
   **When** they are rendered
   **Then** text is rendered as text only (no `dangerouslySetInnerHTML`, enforced by a lint
   rule), and links are only rendered for `http:`/`https:` URLs, with
   `rel="noopener noreferrer"` and `target="_blank"`. A test feeds a `javascript:` URL
   and an HTML title and checks both are inert.
5. **Given** CI
   **When** `make check` runs
   **Then** it also runs the web checks: type check, lint, unit tests and a production
   build; Node version pinned (`.nvmrc` / `engines`), dependencies installed with
   `npm ci` from a committed lockfile; Dependabot watches `web/`.
6. **Given** a production build
   **When** the Go API starts
   **Then** it serves the built page at `/` (embedded files) with a Content-Security-Policy
   that allows only same-origin scripts and styles.

## Out of scope

- Shortlisting, skipping, notes, board view (M6). Accounts, per-user presets, sharing
  (milestone S).

## Security notes

New ecosystem (npm) → supply-chain care: few dependencies, lockfile committed,
`npm audit` in CI at high severity, Dependabot. ADR for the stack. Untrusted content
rules in AC4 and CSP in AC6.
