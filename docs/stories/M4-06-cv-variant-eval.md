# M4-06: Measure the CV-variant choice

- **Status:** Ready
- **Milestone:** M4 — Evidence bank and tailoring
- **Linear:** [HAM-54](https://linear.app/hamid-corp/issue/HAM-54/m4-06-measure-the-cv-variant-choice)
- **Estimate:** ½ day

## Story

As the job seeker, I want to know how often the chosen CV variant matches the one I'd
pick, so that tailoring starts from the right version of my CV.

## Context

M3-06 picks a variant per lead with `decide()`. In the 509-lead run it chose DIST 173
times, against 88 BE, 71 CONS, 66 AI, 56 PAY and 55 FS. That looks skewed for a backend
profile. M4-02 starts each draft from the lead's variant, so it must be measured first.
Roadmap follow-up from M3.

## Owner input

**OI-3**: one variant per eval posting in `agents/evals/variants.yaml`; format in
`docs/stories/M4-owner-input.md`. This story writes the file with every answer
empty (`"?"`). It never shows the model's choice, so the label is blind.

## Acceptance criteria

1. **Given** the eval set (36 postings)
   **When** `make evals-variants SET=init` runs
   **Then** it writes `agents/evals/variants.yaml`, one entry per posting with its
   company and title as a comment and `variant: "?"`. It refuses to overwrite answers
   already given.
2. **Given** the filled file
   **When** `make evals-variants` runs
   **Then** it reports, from the recorded reasons run:
   - a 6×6 confusion matrix of the owner's variant against the model's;
   - the agreement rate;
   - the model's share of each variant.
   No new model call is needed: the reasons run already recorded `cv_variant`. It
   refuses while answers are still `"?"`.
3. **Given** the result
   **When** agreement is below a threshold the owner sets (proposed: 70%)
   **Then** at most two new `decide()` question versions are tried on the 36. Each try
   is a new versioned question, recorded in the run, at about $0.003. Whichever scores
   best is kept, and agreement becomes a regression gate like the extraction gates.
   - If none reaches the threshold, report and stop: the owner decides.

## Out of scope

- Changing which variants exist. Variant-specific evidence selection (M4-02 uses the
  tags).

## Security notes

- Labels aren't personal: they are committed.
- `decide()` keeps getting the posting's title and extracted skills as escaped data;
  no change to what reaches the model.
