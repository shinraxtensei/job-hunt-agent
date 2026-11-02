# M2-09: Classifier country coverage

- **Status:** In review
- **Milestone:** M2 — Rules and a plain tracker
- **Linear:** [HAM-38](https://linear.app/hamid-corp/issue/HAM-38/m2-09-classifier-country-coverage)
- **Estimate:** ½ day

## Story

As the job seeker, I want every country and the Latin America / Americas regions
recognised in posting locations, so that remote roles limited to countries outside my
rules aren't shown as "unknown scope", and I can see which locations are still unplaced.

## Context

M2-08's `matched_location` showed Kraken's LatAm-remote roles as "Panama +21": Panama and
Paraguay weren't in the hand-picked country table (ADR-0011), so they classified as
remote scope `unknown`, which the default preset accepts on purpose. The table is replaced
by the complete ISO 3166-1 list from checked-in files.

## Acceptance criteria

1. **Given** the classifier
   **When** a location names any ISO 3166-1 country by its English name or a common alias
   ("UAE", "Türkiye"/"Turkey", "Czechia", "South Korea", "Côte d'Ivoire"…)
   **Then** it resolves to that alpha-2 code. All 249 codes are read at start-up from
   `internal/job/geo/iso3166-1.csv` (generated once from CLDR by `geo/generate.mjs`,
   reviewed, committed) and `geo/aliases.csv`; nothing is downloaded. "Georgia" stays
   unresolved (it is also a US state).
2. **Given** "Remote - LatAm" or "Remote, Americas"
   **When** classified
   **Then** the remote scope is `latam` or `americas`, not `unknown`; presets accept
   `latam` as a remote scope and `LATAM` / `AMERICAS` as country aliases (like `EU_EEA`).
3. **Given** the change
   **When** `job.ClassifierVersion` is bumped and `collector classify` runs
   **Then** the PR reports how the default preset's shown count changed, and why, with
   examples.
4. **Given** `collector classify --report-unknown`
   **When** run
   **Then** it prints the most common location strings the classifier can't place, with
   job counts (posting text printed quoted). Documented in CLAUDE.md.
5. **Given** the classifier tests
   **When** they run
   **Then** every ISO code resolves, aliases resolve, Panama/Paraguay/LatAm cases pass,
   and the existing classifier tests still pass.

## Out of scope

- More cities (the report shows them; add them when they matter), US state codes ("VA").

## Security notes

Location strings are untrusted: matched against fixed tables only, printed quoted by the
report.
