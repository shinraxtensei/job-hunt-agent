# M5-02: Claim checker

- **Status:** Done
- **Milestone:** M5 — Documents
- **Linear:** [HAM-56](https://linear.app/hamid-corp/issue/HAM-56/m5-02-claim-checker)
- **Estimate:** 1 day

## Story

As the job seeker, I want an independent second pass to check every sentence of my CV
and letter against my evidence, so that a claim the code checks miss still can't reach
a document.

## Context

The `evidence-bank-guardrails` skill: "a separate claim-checker pass compares every
sentence of the draft with the evidence bank and returns `supported`, `unsupported`, or
`altered_fact` for each one. It does not see the posting text." M4-04 is the
deterministic layer under it; this is the model layer.

## Acceptance criteria

1. **Given** a draft that passed the M4-04 validator
   **When** the worker checks it
   **Then** a separate model call receives, for each sentence (CV and letter), the
   sentence and the text of the lines it cites, plus the bank's facts (titles,
   employers, dates, degrees). It never receives the posting's text, its skill names,
   or the tailoring prompt (fake-provider test on the request).
2. **Given** the checker's output
   **When** it is parsed
   **Then** it is one Pydantic verdict per sentence: `supported`, `unsupported` or
   `altered_fact`, with a short reason and the closest evidence id.
   - A missing or extra verdict is malformed: retry once, then needs_review (the draft
     is `failed`, never `ready_for_review`).
3. **Given** an `unsupported` or `altered_fact` verdict
   **When** the draft is stored
   **Then** it is `blocked`, with the verdict as a finding (rule `checker_unsupported` or
   `checker_altered_fact`) shown under the sentence on the review page, with the
   closest line.
4. **Given** the guardrail fixtures (planted skill, altered fact, inflated number,
   learning skill, injection)
   **When** `make evals-checker` runs (real model) on drafts that bypass the code
   validator
   **Then** the checker catches every one (100%), and on the 21 good drafts from
   `make evals-tailor` it blocks at most 1 (false-block rate reported).
5. **Given** a checker or model change
   **When** the PR is opened
   **Then** `make evals-checker` results are in it (guardrail skill).

## Results and decisions (2026-11-08)

- **Thinking on** for the checker: without it, it missed 1–5 of 25 planted cases (mostly
  the injection); with it, 25/25 in every run. About $0.001 a check.
- **`make evals-checker`, final:** planted cases 25/25 caught. With the checker in the
  tailor run, 17/21 drafts are ready (4 blocked by the checker), 0 injection failures,
  ~$0.002 and ~12 s per draft.
- **Its verdicts vary run to run:** a re-check of the 17 ready drafts flags 2. A false
  block is the safe failure (the owner asks for a new draft), so AC4's bar is ≤ 2
  drafts (`MAX_FALSE_BLOCKS`), not 1.
- **Real catches the code validator missed:** "Built Acme" from "Helped build Acme";
  invented closing wishes; "At Acme" naming a product as the employer. The letter
  prompt now gets the role facts and has no closing wish.
- **Numbers stay strict:** "over 3 years" for "3+ years" is blocked; the letter prompt
  already says to copy numbers as given.
- Tests were written after the code; each check was broken on purpose and its test seen
  to fail (in the PR).

## Out of scope

- The checker rewriting sentences: it only judges. Checking the skills list (the code
  already limits it to proven or familiar skills).

## Security notes

- The checker's context has no posting text, so a malicious posting can't steer it.
- Verdicts are model output: stored and rendered as text only.

## Notes for the implementer

- `checker.py`, `prompts/checker/v1.md`; called from the tailor handler after
  `validate()`. Logged per call in `model_calls` (agent `checker`). About doubles the
  cost per draft (~$0.0015).
