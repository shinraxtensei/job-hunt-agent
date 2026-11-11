# M9-03: Stretch is a flag; four fit levels

- **Status:** Done
- **Milestone:** M9 — Cleanup
- **Linear:** [HAM-75](https://linear.app/hamid-corp/issue/HAM-75/m9-03-stretch-is-a-flag-four-fit-levels)
- **Estimate:** 1 day

## Story

As the job seeker, I want "this is a step up" shown apart from how well a lead fits, so
that a strong lead above my level isn't lumped with leads that don't fit.

## Context

"Stretch" was a fit level between good and no (ADR-0021, M3-06); the location gate (M3-09)
capped leads you can't work at to it. ADR-0031 splits it out.

## Acceptance criteria

1. **Given** a lead whose job asks for a higher level than the profile's, or more years
   than it has
   **When** it is scored
   **Then** its score carries `stretch: true`, shown as a "Stretch" tag beside the fit;
   otherwise false (unknown level and no years: false).
2. **Given** any score
   **When** its level is computed (Python, the view, the API)
   **Then** it is one of very strong, strong, good, no; the location gate caps it at good.
3. **Given** the owner's blind labels (five levels)
   **When** the bands are re-fitted on dev
   **Then** a "stretch" label counts as no; the agreement on all 36 is reported, with how
   often the flag matches the owner's stretch labels.

## Out of scope

- Telling the reason agent about the flag.
- Relabelling the eval set.

## Security notes

None: scoring output computed in code from the extraction and the profile; no new input,
output or model data.

## Results (2026-11-11)

- Bands re-fitted on dev: 72 / 52 / 32.5 → `scoring.yaml` 72 / 52 / 33 (whole numbers).
- Agreement with the location gate: dev 26 (fitted) exact 76.9%, within one 96.2%;
  held-out 10 exact 70.0%, within one 100%; **all 36 exact 72.2%, within one level
  100%** (five levels before: 55.6% and 91.7%).
- The stretch flag is on 6 of the owner's 7 stretch labels, and on 13 of the other 29
  (step-ups the owner still rated good or strong).
- Reason prompt v4: the cap line says "FIT CAPPED" without a level; `make evals-reasons`
  run, see the PR.
- Tests: `test_stretch_flag`, `test_stretch_labels_count_as_no`, the location-gate table
  (Python and Go), the view test, `TestFitLevel`, vitest "tags a lead that asks for more
  than the profile"; API goldens carry `stretch`.
- Local data, 584 scored leads re-scored (`SCORE_CODE` 9): very strong 7 → 10, strong
  106 → 113, good 157 → 254, stretch 120 → (flag), no 194 → 207. The flag is on 337 of
  584 (the profile is mid level with 3 years; most roles ask for senior): 7 very strong,
  52 strong, 144 good, 134 no.

