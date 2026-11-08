# M5-01: Cover letter agent

- **Status:** Ready
- **Milestone:** M5 — Documents
- **Linear:** [HAM-55](https://linear.app/hamid-corp/issue/HAM-55/m5-01-cover-letter-agent)
- **Estimate:** 1 day

## Story

As the job seeker, I want a cover letter of under 250 words for a lead, made with the CV
draft and built only from my evidence, so that I can send a matching pair without fixing
invented claims.

## Context

PRD: "CV + cover letter (< 250 words) from the evidence bank". The
`evidence-bank-guardrails` skill applies in full; a letter is held to the same rules as
the CV.

It reuses M4 as is:
- the draft request (ADR-0025): one request makes the CV and the letter, in the same
  `drafts` row, under one status;
- coverage (M4-03) and evidence selection (M4-02);
- the validator (M4-04): every letter sentence carries `evidence_ids` and is checked
  like a bullet.

## Owner input

**OI-6** (`docs/stories/M5-owner-input.md`): what a letter may say beyond evidence lines.
Default: only the lead's company name and job title (filled in by code), plus an
optional `letter` section in `evidence.yaml` with checked lines the owner writes
(motivation, availability, work authorisation). **OI-7**: language (default English).

## Acceptance criteria

1. **Given** a draft request
   **When** the worker makes the draft
   **Then** it also makes a letter, from a second model call. The model receives:
   - the same selected lines as the CV, plus the `letter` lines;
   - the job's extracted skill names as escaped data.
   It never receives the posting's text, the free-text profile or the whole bank
   (fake-provider test on the request).
2. **Given** the model's output
   **When** it is parsed
   **Then** it is a Pydantic letter: 2–4 paragraphs of sentences, each with
   `evidence_ids`.
   - Code adds the greeting, the company name and job title (from the lead record), the
     sign-off and the owner's name. The model writes none of them.
   - Malformed output: retry once, then needs_review.
3. **Given** the letter
   **When** it is counted
   **Then** it is under 250 words, greeting and sign-off included. A longer one is
   shortened on the retry; if still too long, the draft is `blocked` with the rule
   `too_long`.
4. **Given** the letter
   **When** it is validated
   **Then** each sentence goes through the M4-04 checks (unsupported, altered fact,
   inflated number, learning as experience, familiar skill in a sentence, scope
   inflation). Any violation blocks the whole draft (CV and letter).
5. **Given** the eval set
   **When** `make evals-tailor` runs
   **Then** it reports letters too: blocked by rule, word counts, and the injection
   fixtures (substance identical to the clean baseline). The guardrail cases stay at
   100%.
6. **Given** the review page (M4-05)
   **When** a draft is shown
   **Then** the letter is shown under the CV, with each sentence's evidence and
   findings, the same way as bullets.

## Decisions (owner, 2026-11-08)

- OI-6, OI-7, OI-9, OI-10: the defaults in `M5-owner-input.md`.
- English only for now. French postings (M7-04) will need French evidence lines written
  and checked by the owner; evidence is never machine-translated.

## Out of scope

- Hand edits to the letter (OI-9). PDF (M5-03). Per-company research or "why this
  company" text not in the evidence bank: never.

## Security notes

- Same trust boundary as M4-02: the posting reaches the model only as extracted skill
  names, escaped. Company name and title are inserted by code, never by the model.
- Letter content is personal: never logged; stored only in `drafts`.

## Notes for the implementer

- `drafts.letter jsonb` (new column, migration); ADR-0025 gets an amendment line.
- `prompts/letter/v1.md`; `LetterContent` in `drafts.py`; `validate()` takes a list of
  sentences, so the letter can reuse it by turning paragraphs into `Bullet`s.
