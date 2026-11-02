# 0009. Group duplicate jobs by company and a title key stored on `jobs`

- **Status:** Accepted
- **Date:** 2026-11-02
- **Story:** docs/stories/M2-02-deduplication.md

## Context

ADR-0008 made `jobs` the normalised record and said M2-02 extends it under its own ADR.
Duplicates today are within one company: the same role posted once per city or region
("Solutions Architect - China", "- Germany"), or exact repeats (489 titles appear more than
once within a company, 1,479 records, in the local data on 2026-11-02). A title's suffix is
often a *team*, not a place ("Engineering Manager - Android", "Staff Engineer, Compute
Foundation", "Theodo Data & IA" vs "Theodo FinTech"), so "drop everything after the dash"
would merge different roles. Titles are untrusted text.

## Decision

- **`jobs.title_key text NOT NULL`**, set by `job.TitleKey(title, locations)` when the record
  is normalised (so in the posting's transaction, ADR-0008). A **group** is all jobs with the
  same `(company_id, title_key)`; indexed on that pair.
- **`TitleKey`:** fold accents (fixed table), lowercase, punctuation and whitespace collapsed.
  A trailing segment (after ` - `, ` – `, ` — `, ` | `, `, `, or in final parentheses) is
  removed only if, after dropping words from a **fixed vocabulary** (work modes: remote,
  hybrid, onsite, télétravail…; regions: EMEA, AMER, APAC, LATAM, Benelux, Nordics, DACH…;
  gender markers H/F, F/H, m/w/d; fillers like "possible", "depuis", "complet"), what is left
  is empty or appears as whole words in one of **the record's own `locations`**. Everything
  else stays, including seniority words. No regular expression or pattern is ever built from
  a title; location matching is a token comparison against the same record's data.
- **`job_groups` view:** per group, the member count, the **canonical** posting (the one seen
  first; once M2-04 marks closed postings, the open one seen first), and the union of members'
  locations and workplaces. Derived on read, so groups can't go stale, and nothing is
  written for a company that wasn't collected. Python reads the same view later.
- `job.NormaliserVersion` goes to **2**: `collector normalise` fills `title_key` for every
  existing record.

**Team and department are not part of the key** (checked against the local data on
2026-11-02). Of the 616 groups with more than one member, 70 (237 records) have members
with a different team or department: 17 by team, 57 by department (Lever 6/24, Greenhouse
49/414, Ashby 15/178). After dropping region words, 68 still differ, but mostly because
the department is a regional cost centre or an internal label for the same role posted in
several regions: Stripe "Account Executive, SMB Grower" under `1175 Enterprise - Account
Executives (NA)` / `1185 Account Executives (EMEA)` / `1195 Account Executives (APAC)`;
Thoughtworks `1Y_TWDEU SL DATA` / `1Y_UKSTH SL DATA`; Cloudflare "Field Sales" / "Sales";
GitLab "AMER - Enterprise" / "APAC - Enterprise". A few are genuinely different teams
(Theodo "AI-Build" / "AI-Modernisation", Stripe "Product Sales - Billing" / "- Metronome").
Adding team and department to the key would take the data from 4,463 to 4,552 groups,
undoing 89 of the 1,410 merges, most of them correct per-region duplicates.

## Alternatives considered

- **Team and department in the key:** separates the few real team splits, but undoes ~89
  correct per-region merges because departments carry regions and cost-centre codes.

- **Strip any suffix after a separator:** merges teams and territories (Adyen's 11
  "Engineering Manager - …" roles would become one).
- **Fuzzy matching (edit distance, embeddings):** non-deterministic boundaries, hard to test
  with a pair table, and the story keeps fuzzy matching out of scope.
- **A fixed list of cities and countries:** large, never complete, and the record already
  says where it is.
- **A `job_groups` table:** has to be rewritten on every change and can drift; the view is
  always current.

Merges are made visible rather than perfect: every lead shows how many postings it folds
and links to all of them (M2-05 `GET /api/leads/{id}/members`, M2-06 "N postings"), so a
wrong merge is one click from being noticed.

## Consequences

- **Good:** deterministic, explainable groups; teams, territories, stacks and seniority stay
  apart; the pair table pins behaviour.
- **Bad:** a place named in the title but not in the record's locations is not stripped
  ("- Chicago" with location "Illinois", "- Bretagne" with location "Brest"), so those copies
  stay separate. That errs towards showing a duplicate rather than hiding a different role.
- **Revisit when:** missed duplicates are common in the tracker (add region names to the
  vocabulary, or match US states and city names from a source), or cross-company duplicates
  appear (agencies).
