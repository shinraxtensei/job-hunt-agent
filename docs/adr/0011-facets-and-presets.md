# 0011. Job facets on `jobs`, a classifier version, and YAML filter presets

- **Status:** Accepted
- **Date:** 2026-11-02
- **Story:** docs/stories/M2-03-classify-and-filter.md

## Context

M2-03 replaces "rules drop postings" with "classifiers tag, presets hide". The owner wants
to widen filters at will and to share the tracker with friends in other fields, so nothing is
deleted and every hidden job must say why. ADR-0008 made `jobs` the normalised record and
said M2-03 extends it under its own ADR.

The local data (5,873 postings, 2026-11-02) sets the constraints: departments are noisy
("Field Sales", "SME & Growth Business", "All Cost Center", Stripe's "1185 Account
Executives (EMEA)"); seniority words open most titles ("senior" 1,378, "staff" 278,
"principal" 185, "head" 84, "lead" 61, "Sr." 24); many titles are French ("Spécialiste",
"Conseiller(ère)", "Architecte", "Stage de fin d'études", "Candidature spontanée"); remote is
often written "Home Based - Worldwide/EMEA/Americas" or "Distributed, Global"; only 8
postings are in Casablanca; requirement text sits in Lever's `lists`, Greenhouse's
HTML-escaped `content` and Ashby's `descriptionPlain`.

## Decision

**Facets are columns on `jobs`**, written with the job record (same transaction as the
posting, ADR-0008) and rebuilt by `collector classify`:

| Column | Type | Values |
|---|---|---|
| `role_family` | text NOT NULL, CHECK | `backend` `frontend` `fullstack` `mobile` `platform` `data` `ai_ml` `security` `qa` `eng_management` `product` `design` `sales` `marketing` `customer` `operations` `finance` `people` `legal` `other` `unknown` |
| `seniority` | text NOT NULL, CHECK | `intern` `junior` `mid` `senior` `staff_plus` `lead_manager` `unknown` |
| `experience_min_years` | integer NULL | an explicit minimum from the description; NULL when none |
| `remote_scopes` | text[] NOT NULL DEFAULT '{}' | **every** remote scope the job's locations name: `worldwide` `emea` `europe` `africa` `americas` `apac` `us_only` `country:<CODE>` (ISO alpha-2, upper case) `unknown`; empty when no location is remote. A remote rule passes if **any** scope is accepted |
| `countries` | text[] NOT NULL DEFAULT '{}' | ISO alpha-2 codes: the board's codes (Lever) plus country and city names found in `locations` via fixed tables |
| `classifier_version` | integer NOT NULL DEFAULT 0 | `job.ClassifierVersion` that produced the facets |

Cities stay in `locations`; a preset's cities are matched against them as whole words
(the same token comparison as ADR-0009). A fixed city→country table, built from the cities
in the local data (classifier version 2, owner request), fills `countries` for bare-city
locations ("Amsterdam", "NL - Amsterdam") so country rules such as relocation can match.

**Classifiers** are pure functions in `internal/job` with fixed word tables, title first,
then team/department; `unknown` is a normal answer and nothing is guessed. The tables are
built from the local data's most common titles, teams and departments, and pinned by 60+
real titles (English and French, sales and support included). Descriptions and titles are
untrusted: fixed precompiled patterns only, never logged. `job.ClassifierVersion` is
separate from `job.NormaliserVersion`, so tuning a classifier re-classifies without
re-normalising.

**Presets** are YAML files in `data/presets/<name>.yaml`, validated like the watch list
(every invalid line reported before anything runs):

```yaml
name: default
description: My profile (resume, 2026-11)
role_family: [backend, fullstack, frontend, mobile, platform, data, ai_ml, unknown]
seniority: [junior, mid, senior, unknown]
experience_max_years: 5        # hide only an explicit minimum of 6 years or more
closed: hide                   # hide | show | only
location:                      # shown if ANY rule matches
  - {workplace: [remote], remote_scopes: [worldwide, emea, africa, europe, "country:MA", unknown]}
  - {workplace: [onsite, hybrid], cities: [Casablanca, Rabat]}
  - {workplace: [onsite, hybrid], track: [relocation], countries: [EU_EEA, GB, CH, AE]}
  - {workplace: [unknown]}
```

A facet list *includes* values; an omitted key means "no filter". `EU_EEA` is a fixed
alias for the EU/EEA country codes. **Evaluation** is a pure function `preset.Evaluate`:
it returns `shown`, or `hidden` with every filter that hid the job (`role_family=sales`,
`seniority=staff_plus`, `experience_min_years=7`, `location`, `closed`). It runs at query
time (the API builds its query from it in M2-05), so editing a preset needs no recompute.
It evaluates **duplicate groups** (ADR-0009): role family, seniority and experience from
the canonical job; location passes if any member's location rule passes ("Berlin, Remote"
shows if Remote qualifies).

`collector classify` rewrites facets where `classifier_version` is older than
`job.ClassifierVersion` (and the normal write path keeps new and changed jobs current);
`collector preview <preset>` prints shown/hidden counts by filter and samples, for tuning.

## Alternatives considered

- **A `job_facets` table:** one more join for every query, and facets are 1:1 with jobs.
- **Storing preset results:** every preset edit would need a recompute, and friends' presets
  would multiply it; evaluation is cheap at query time.
- **Rules in Go code:** changing a default would mean a code change; presets are data, like
  the watch list.
- **A model for role family:** non-deterministic and not testable with a table; M3 is where
  models come in.

## Consequences

- **Good:** one shape for the API, the tracker and friends' presets; every hidden job says
  why; classifiers can improve and re-run without touching raw data.
- **Bad:** word tables need care as new boards and languages appear; a job whose title is
  vague ends up `unknown` and is shown by the default preset, so the list is longer than a
  guessing classifier would make it.
- **Revisit when:** `unknown` role family exceeds ~15% of jobs, or a friend's field needs
  families this list doesn't have.

## Owner decisions (2026-11-02)

- `remote_scopes` is a list (a posting can be remote from several countries or regions);
  tested with a multi-country Ashby posting.
- The default preset accepts `country:MA` remote scopes ("Remote – Morocco").
- `role_family: unknown` stays in the default preset ("when unsure, show").
- `experience_max_years: 5` encodes "hide only when the explicit minimum is 6 or more".
