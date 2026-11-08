# M4-02: Tailoring agent

- **Status:** Done
- **Milestone:** M4 — Evidence bank and tailoring
- **Linear:** [HAM-50](https://linear.app/hamid-corp/issue/HAM-50/m4-02-tailoring-agent)
- **Estimate:** 1 day

## Story

As the job seeker, I want to ask for a tailored one-page CV draft for a lead and get it
within two minutes, built only from my evidence, so that I can apply to the good leads
quickly without fixing invented claims.

## Context

PRD: "Tailored one-page CV ready within 2 minutes of asking"; "tailoring may select,
reorder and rephrase evidence; never add skills, numbers, titles or dates".

It builds on:
- the evidence bank (M4-01);
- coverage (M4-03);
- the validator and draft schema (M4-04);
- the CV variant (M3-06, measured in M4-06).

The `evidence-bank-guardrails` skill applies in full. Drafts are requested the way
reasons are (ADR-0021, ADR-0022): a same-origin POST marks the lead, and the worker
queues the work.

## Owner input

**OI-4**: the one-page layout in `data/profile/cv_layout.yaml`; format in
`docs/stories/M4-owner-input.md`. Without it the defaults shown there are used and the
owner adjusts them on the first draft (M4-05). **OI-1** must be imported (M4-01) before
a real draft can be made.

## Acceptance criteria

1. **Given** a lead and the evidence bank
   **When** a draft is requested (`POST /api/leads/{id}/draft`: refused cross-site and
   for a Host other than loopback, as in ADR-0022)
   **Then** the worker selects evidence in code, and the model receives only those
   lines. Selection:
   - covered lines for the lead's skills, then lines tagged with its variant;
   - within the layout's limits (roles, bullets per role, projects).
   The model never receives the free-text profile, the whole bank, or the posting's
   text. It gets the posting's extracted skill names as escaped data (fake-provider
   test on the request).
2. **Given** the model's output
   **When** it is parsed
   **Then** it is a Pydantic draft: summary sentences and bullets, each with
   `evidence_ids`, plus the skills list.
   - Code fills in the header and the exact facts (titles, employers, dates, degrees)
     from the evidence bank.
   - The model can't change a fact even if it writes one.
   - Malformed output: retry once, then needs_review.
3. **Given** the draft
   **When** it is stored
   **Then** it goes through the M4-04 validator and is stored with:
   - its violations and its coverage;
   - the versions it came from (evidence, profile, prompt, model);
   - the lead's content hash.
   It is `blocked` or `ready_for_review`, never sent. Each request makes a new draft
   version; older ones are kept.
4. **Given** the eval set
   **When** `make evals-tailor` runs (real model, cost estimated first, under $0.20)
   **Then** it drafts for a fixed set of eval postings plus the injection fixtures. It
   reports:
   - drafts blocked, by rule;
   - the guardrail cases, which must be at 100%;
   - substance identical to the clean baseline for each injection fixture;
   - cost and time per draft (target under 2 minutes).
   Prompt or model changes must not lower it (guardrail skill).
5. **Given** a one-page layout
   **When** the draft is built
   **Then** it respects the limits (roles, bullets, projects, skills count) and the
   section order. A bullet over 2 lines (about 200 characters) is shortened by the
   model on the retry, or the line is dropped. Never cut mid-claim.

## Results and decisions (2026-11-08)

- **`make evals-tailor`, prompt v1, owner's real bank:**
  - 21 drafts: 17 for postings labelled good or better, plus 4 injection fixtures.
  - All 21 `ready_for_review`; 0 injection failures.
  - 3.9 s per draft (max 4.6 s); $0.0137 for the run.
- **The first run blocked 3 drafts.** The model put Go (learning), Tookan and Infobip
  (not in the skill map) in the skills section. Two fixes:
  - the prompt now names the skills the section may list;
  - rendering drops any other entry. Dropping adds nothing, so rule 1 holds.

  Sentences are never altered by code, only validated.
- **Approve and discard** come with the review page (M4-05). Draft requests reuse the
  ADR-0022 write guards (ADR-0025).

## Out of scope

- Cover letter (M5-01), model claim checker (M5-02), PDF (M5-03). Editing a draft by
  hand (M5).

## Security notes

- Golden rule 3 and the six guardrail rules apply in full.
- The model sees only selected evidence lines and extracted skill names, inside the
  escaped data block. Injection can't reach the evidence selection, which is done in
  code.
- Drafts contain CV content: never logged. They are stored in Postgres on the owner's
  machine.
- ADR: a `drafts` table, a `tailor` work kind and a second API write, so an ADR is
  required.
