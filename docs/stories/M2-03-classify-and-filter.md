# M2-03: Classify jobs and filter them, with my profile as the default

- **Status:** Ready
- **Milestone:** M2 — Rules and a plain tracker
- **Linear:** [HAM-32](https://linear.app/hamid-corp/issue/HAM-32)
- **Estimate:** 1½ days

## Story

As the job seeker, I want every job tagged with its role family, seniority, location and
required experience, and a filter preset that shows only what fits my profile by default,
so that my daily list is short, while nothing is thrown away and anyone can widen the
filters (or use another preset) with one click.

## Context

This replaces the earlier "hard rules drop postings" design. Dropping is the wrong model:
the owner wants to widen filters at will, and plans to share the tracker with friends in
other fields (sales, marketing...), who need the same data with different filters. So
rules **tag**, filters **hide**, and nothing is ever deleted or marked dropped.

The ~5,870 stored postings span every department of 50 companies (engineering, sales,
support, legal...), so the role-family tag is what makes the list usable.

Owner defaults come from the resume (2026-11): Software Engineer since May 2023 (~3.5
years), TypeScript/NestJS backend, React, Flutter, PostgreSQL, distributed systems, AI
pipelines; based in Casablanca, open to remote.

## Acceptance criteria

1. **Given** a canonical job record (M2-01, M2-02)
   **When** it is classified
   **Then** it gets these facets, each with `unknown` when the data can't tell:
   - `role_family`: engineering families `backend`, `frontend`, `fullstack`, `mobile`,
     `platform` (infra/SRE/DevOps), `data`, `ai_ml`, `security`, `qa`, `eng_management`;
     other families `product`, `design`, `sales`, `marketing`, `customer` (success/support),
     `operations`, `finance`, `people`, `legal`, `other`.
   - `seniority`: `intern`, `junior`, `mid`, `senior`, `staff_plus` (staff, principal,
     distinguished), `lead_manager` (lead, head of, manager, director, VP), `unknown`.
   - `experience_min_years`: an explicit minimum stated in the description, else none.
   - `remote_scope`: for remote jobs, `worldwide`, `emea`, `europe`, `africa`, `americas`,
     `us_only`, `country:<code>` or `unknown`.
   - plus what M2-01/M2-04 already provide: workplace, countries, cities, company track,
     closed.
2. **Given** the classifiers
   **When** they are table-tested
   **Then** role family and seniority use title, then team/department, with at least 60
   real titles from the local database (English and French, e.g. "Ingénieur Backend
   Senior", "Account Executive", "Staff SRE", "Développeur Full-Stack (H/F)"), and the
   experience extractor passes the phrasing table: "5+ years", "at least 6 years",
   "minimum de 5 ans", ranges use the lower bound, and traps are ignored ("5 years of
   growth", "founded 10 years ago").
3. **Given** a filter preset file in `data/presets/` (YAML, like the watch list)
   **When** it is loaded
   **Then** it can include or exclude any facet value, set a maximum for
   `experience_min_years`, list accepted cities, countries and remote scopes, and show or
   hide closed jobs. Invalid entries are reported with line numbers before anything runs.
4. **Given** the owner's preset `data/presets/default.yaml`
   **When** the tracker opens without explicit filters
   **Then** it applies these defaults (each one a filter, not a deletion):
   - role families: backend, fullstack, frontend, mobile, platform, data, ai_ml
   - seniority: junior, mid, senior, unknown (intern, staff_plus and lead_manager hidden)
   - experience: hide only when an explicit minimum is 6 years or more
   - location: remote with scope worldwide, emea, africa, europe or unknown; onsite or
     hybrid in Casablanca or Rabat; for `relocation`-track companies, onsite or hybrid in
     the EU/EEA, UK, Switzerland or UAE; workplace `unknown` is shown
   - open jobs only
5. **Given** any job and a preset
   **When** the preset is evaluated
   **Then** the result says shown or hidden, and for hidden jobs **which filters hid it**,
   so the owner can audit the defaults ("show me what my defaults hide, and why").
6. **Given** a classifier change or a new collector run
   **When** `collector classify` runs (and after every collector run)
   **Then** facets are recomputed for changed records, with the classifier version stored
   so a version bump re-classifies everything. Presets are evaluated at query time, so
   editing a preset needs no recompute.
7. **Given** the local database
   **When** classification has run
   **Then** the PR shows counts per role family and seniority, how many jobs the default
   preset shows vs hides (by filter), and 10 random hidden jobs with their reasons for a
   manual check.

## Out of scope

- Scoring and ranking against the CV (M3). Visa and relocation support read from
  descriptions (M3 extraction); in M2, relocation comes from the company's track.
- Per-user presets, accounts and sharing (milestone S).

## Security notes

Titles and descriptions are untrusted: matched with fixed, precompiled patterns only,
never logged, never used to build patterns. Preset files hold preferences, not secrets.

## Notes for the implementer

- Classifiers are pure functions in `internal/job` with big table tests; facets live on
  the job record (or a 1:1 table, per the M2-01 ADR). Preset evaluation is a pure
  function too, used by the API (M2-05) to build its query.
- Keep `when unsure, show`: a filter only hides what it can show is out (`unknown` passes
  location and seniority filters unless a preset explicitly hides `unknown`).
