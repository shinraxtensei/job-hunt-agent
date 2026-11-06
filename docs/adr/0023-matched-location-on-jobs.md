# 0023. Reconcile stores the default preset's matched location on jobs, for the scorer

- **Status:** Accepted
- **Date:** 2026-11-06
- **Story:** docs/stories/M3-09-location-gate.md

## Context

The location factor's detail feeds the reason ("You can't work there: it's on-site in
…"). The scorer (Python) named the job's first location. For postings with several
locations, that is often not the one that made the lead shown:
- Cloudflare showed "Austin" where London qualified it.
- Elastic showed "Canada" where Greece qualified it.

The tracker already knows the right one: M2-08's `matched_location`, the location that
passed the default preset's location rules. But only the Go API computes it, at read
time, with the Go classifier and preset code. Python has neither, and porting them
would duplicate a large piece of Go.

## Decision

- **`jobs.matched_location`** (text, `''` when no location rule applied) holds the
  default preset's matched location for the canonical posting of each shown group.
- **The reconcile step writes it** (Go, `SetMatchedLocations`), in the same pass that
  queues extract items for the leads the default preset shows (ADR-0001). It touches
  only rows whose value changed.
- **The scorer reads it** (`JOB_SQL`). The location detail names it first, and falls
  back to the job's own first location (skipping entries that only name a workplace,
  such as "Hybrid").
- **A data contract between Go and Python** (written by Go, read by Python). The Go
  integration test (`TestReconcileStoresMatchedLocation`) and the Python table test
  pin both sides.

## Alternatives considered

- **Port the classifier and preset rules to Python:** two implementations to keep in
  step.
- **Pick the location in Python from the country codes:** `jobs.locations` and
  `jobs.countries` aren't aligned element by element, so the guess fails exactly on
  multi-location postings.
- **Let the API rewrite the detail at read time:** fixes the breakdown, not the reason
  the model already wrote.

## Consequences

- **Good:** the reason and the breakdown name the place the owner can work from. One
  implementation of the preset rules.
- **Bad:**
  - The value is only as fresh as the last reconcile (`collector run`, `add`,
    `enqueue`).
  - A change of matched location alone doesn't re-score a lead; the next scoring-code
    or profile change does.
  - It reflects the default preset only.
- **Revisit when:** scores become per preset (S: sharing), or the factor itself (not
  only its detail) should use the group's matched member.
