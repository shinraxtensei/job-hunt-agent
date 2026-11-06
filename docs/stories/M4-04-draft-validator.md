# M4-04: Draft validator and the guardrail tests

- **Status:** Ready
- **Milestone:** M4 — Evidence bank and tailoring
- **Linear:** [HAM-52](https://linear.app/hamid-corp/issue/HAM-52/m4-04-draft-validator-and-the-guardrail-tests)
- **Estimate:** 1 day

## Story

As the job seeker, I want every tailored draft checked in code against my evidence
before I see it as ready, so that a draft that adds, inflates or alters anything is
blocked, whatever the model wrote.

## Context

The `evidence-bank-guardrails` skill lists six tests that must exist and stay green. The
M4 exit criteria require them. This story builds the deterministic validator and those
tests before the tailoring agent (M4-02), test-first: M4-02 must pass it. The
independent model-based claim checker (sentence by sentence, without the posting) comes
in M5-02. This validator is the code layer under it, and it never calls a model.

It defines the draft schema M4-02 produces:
- a header and exact facts (roles, titles, employers, dates, degrees), rendered by code
  from the evidence bank and never written by the model;
- a summary of sentences, each with `evidence_ids`;
- per role and project, bullets of `text` plus `evidence_ids`;
- a skills list.

## Acceptance criteria

1. **Given** a draft
   **When** it is validated
   **Then** it gets a list of violations and warnings, each with its rule, the offending
   sentence and the closest evidence line. A draft with any violation is `blocked`; one
   with none is `ready_for_review` (warnings don't block). Nothing is ever `sent`
   (rule 6).
2. **Every bullet has evidence IDs that exist.** A bullet or summary sentence with no
   `evidence_ids`, an unknown id, a retired line, or a line from another role is
   `unsupported`.
3. **Planted skill.** A posting requires a skill absent from the evidence bank: a draft
   that names it is `unsupported`. The coverage report (M4-03) lists it as a gap.
4. **Altered fact.** A bullet with a job title, an employer, a year or a month-year not
   in its cited lines and roles is `altered_fact`. Test: "Senior Software Engineer"
   for a "Software Engineer" role, "2022" for a 2023 start.
5. **Inflated number.** Every number in a bullet must be among the normalised numbers of
   its cited lines. Test: "~3 years" becoming "5+ years" is blocked, and so are
   "40%" → "50%" and "6 minutes" → "5 minutes". A pure reformat passes ("40 percent" for
   "40%").
   - Digits that are part of a skill or product name are not numbers: OAuth2, S3,
     Python 3, GPT-4, K8s, Web3, 3DS.
   - A digit token is a name when it is glued to letters (`S3`, `K8s`, `GPT-4`), or
     when it follows a skill-map or tech-alias name as its version (`Python 3`,
     `Java 21`).
   - Table tests for each, and for a real number next to a name ("cut Python 3 build
     time by 40%" keeps the 40%).
6. **Skills named in a bullet.** A bullet may name a skill only if one of its cited
   evidence lines names or tags it, at any level.
   - Levels apply only to a skill named in a bullet but not in its cited lines:
     - learning or missing → `unsupported` (blocked);
     - familiar → `familiar_in_bullet` (blocked: skills section only).
   - A learning skill can therefore appear only in a project bullet whose project line
     names it (M4-01).
   - The summary may never name a learning skill. The skills section lists proven and
     familiar skills only, never learning or missing ones.
   - Test: "Built a job collector in Go" passes under a project line that tags Go
     (learning), and is blocked under a role, in the summary, or in the skills section.
7. **Scope inflation (a warning, not a block).** A bullet that uses an ownership or
   leadership verb its cited lines don't support gets `scope_inflation`. The verbs, from
   weakest to strongest:
   - 1: owned (FR: possédé, responsable de);
   - 2: architected (FR: architecturé, conçu l'architecture);
   - 3: led, spearheaded, headed (FR: dirigé, piloté, mené);
   - 4: managed (FR: géré, encadré, managé).

   The warning fires when the bullet's strongest verb outranks every such verb in its
   cited lines. Inflected forms count (lead, leading, led). M4-05 shows the warning;
   the draft stays `ready_for_review`. Table tests in EN and FR.
8. **Prompt injection.** The tailoring input built from an injected extraction (the M3
   injection texts as skill names) leaves the selected evidence and constraints
   unchanged. The skill is a gap marked `suspicious` (M4-03), and the draft request
   carries a flag that M4-05 shows. This is a fake-provider test here; M4-02's eval
   adds the real-model check.
9. **Given** the validator
   **When** `make check` runs
   **Then** all of the above are table tests with fixed drafts and evidence (no model),
   and they are the gate on any change to tailoring code or prompts.

## Out of scope

- The model claim checker (M5-02). Cover letters (M5-01).

## Security notes

- This is golden rule 3's enforcement point. It must never be bypassed: drafts reach
  the page only through it.
- Violations quote draft text (model output) only as data in the DB and the page,
  never in logs.
