# M9-06: Claim checker: an unsupported sentence blocks when two runs agree

- **Status:** Done
- **Milestone:** M9 — Cleanup
- **Linear:** [HAM-78](https://linear.app/hamid-corp/issue/HAM-78/m9-06-claim-checker-block-only-when-two-runs-agree)
- **Estimate:** ½ day

## Story

As the job seeker, I want a draft blocked only when the claim checker is sure, so that I
stop throwing away honest drafts because one run of the checker had a bad moment.

## Context

The claim checker (M5-02) is a model: its verdicts vary from run to run. Re-checking the
same ready drafts flagged 0–3 sentences per run (M5-02, M8-03); planted dishonest cases
were caught 25/25 every time. The code validator (M4-04) runs first and is deterministic;
it is unchanged.

## Acceptance criteria

1. **Given** a draft the code validator passed
   **When** the first checker run flags no sentence
   **Then** the draft is not blocked, and no second call is made.
2. **Given** a first run that flags a sentence as unsupported
   **When** a second run is made
   **Then** it blocks only if the second run flags it too (either verdict); the finding
   shown is the first run's.
3. **Given** a first run that flags an altered fact (title, employer, date)
   **When** the draft is judged
   **Then** it blocks on that run alone: these are what a background check compares.
4. **Given** the eval (`make evals-checker`)
   **When** it runs
   **Then** the planted guardrail cases stay 25/25, and the false-block rate on the ready
   drafts is reported.

## Out of scope

- Changing the checker prompt or model.

## Security notes

This touches the honesty guardrails (evidence-bank-guardrails): a real problem that only
one run catches would no longer block. The planted cases (invented skills, changed titles
and dates, inflated numbers, learning skills as experience, injection) must stay 25/25;
the code validator still blocks its rules on its own; the owner still reads and approves
every draft. The checker still never sees the posting.

## Results (2026-11-11)

- **First version (any flag needs two runs):** planted cases 25/25 in two runs and
  **24/25 in one**: an altered fact flagged by only one run went through. Not shipped.
- **Shipped version (an altered fact blocks on one run; an unsupported sentence needs two):**
  three `make evals-checker` runs on the 19 ready drafts of the last `evals-tailor`: planted
  cases **25/25, 25/25, 25/25**; false blocks **0, 0, 0** (single run this week: 3, 0, 0
  and 1, 0). About $0.065 a run; a second call is made only for drafts with an unsupported
  verdict.
- Tests: `test_two_runs_must_agree` (seven cases), the existing checker and tailor tests
  updated for the second run; the guardrails skill's enforcement note updated.

