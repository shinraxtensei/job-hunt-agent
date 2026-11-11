# 0031. Four fit levels; "stretch" is a flag on the score

- **Status:** Accepted
- **Date:** 2026-11-11
- **Story:** docs/stories/M9-03-stretch-flag.md
- **Amends:** ADR-0021 (what a stored score holds), the M3-09 location gate

## Context

"Stretch" was the fourth of five fit levels, between good and no. It mixed two questions:
how well the lead fits, and whether it is a step up. On the owner's blind labels the
"stretch" leads scored 17–32, with the "no" leads; the band itself had shrunk to half a
point (32–33). A lead can be a strong fit and a step up at once.

## Decision

- **Four levels:** very strong, strong, good, no. The bands are `very_strong`, `strong`,
  `good` (`scoring.yaml`); below good is no.
- **`scores.stretch`** (boolean, written by the worker): the job asks for a higher
  seniority level than the profile's, or more years than it has (from the extraction,
  else the job's facets). Unknown level and no stated years: not a stretch. The API
  returns it on each lead (`stretch`), and the tracker shows a "Stretch" tag beside the fit.
- **The location gate caps at good** (was stretch): a lead you can't work at never ranks
  strong, and isn't buried as no.
- **The blind labels keep their five values.** For fitting and agreement, a "stretch"
  label counts as no (`evals.fit.four_levels`).
- The `lead_scores` view, `score.fit_level` and the API's `fitLevel` share the same rule;
  `SCORE_CODE` 9 re-scores every lead once, and the reason prompt (v4) names the cap
  without a level.

## Consequences

- The API's `fit` filter takes four values; an old URL with `fit=stretch` is refused with
  the usual 400 naming the parameter.
- Skip records keep the fit level they had when skipped (five-level ones included).
