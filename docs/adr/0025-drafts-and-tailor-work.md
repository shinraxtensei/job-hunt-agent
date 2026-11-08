# 0025. Drafts: requested through the API, made by a `tailor` work item, validated before shown

- **Status:** Accepted (follows ADR-0021 and ADR-0022; recorded under the owner's
  working mode of 2026-11-08)
- **Date:** 2026-11-08
- **Story:** docs/stories/M4-02-tailoring-agent.md

## Context

M4-02 makes a tailored CV draft for a lead when the owner asks for one, in under two
minutes, and only from evidence. M4-05 shows it. A draft has to keep:
- what it was made from: the posting content, evidence, profile, prompt and model;
- its coverage (M4-03) and the validator's findings (M4-04);
- its decision, approved or discarded.

The owner may ask again, and old versions stay viewable.

## Decision

The same pattern as reasons (ADR-0021) and the reason request (ADR-0022):

- **`drafts`:** one row per request (a new version each time), with:
  - its status: `requested`, `blocked`, `ready_for_review`, `approved`, `discarded` or
    `failed`;
  - the rendered content, coverage, findings and versions.
- **The API's second write** is `POST /api/leads/{id}/draft`. It inserts a `requested`
  row and nothing else. The ADR-0022 guards apply: loopback Host, cross-origin
  protection, JSON content type. Approve and discard are the third and fourth writes,
  under the same guards (M4-05).
- **The worker queues `tailor` items** for requested drafts, as it does for reasons.
  Claiming, retries and needs_review work as in ADR-0001.
- **The handler** works in four steps:
  1. it selects evidence in code;
  2. it calls the model with the selected lines and the posting's extracted skill
     names only;
  3. it fills in the exact facts from the bank;
  4. it validates the draft (M4-04) and stores it.

  It never sends anything.

## Alternatives considered

- **Draft every scored lead in the background:** spends model budget on leads the
  owner never opens. Lazy, like reasons.
- **Make the draft synchronously in the API:** the API would need the model client,
  the evidence bank and the validator, all of which live in Python.

## Consequences

- **Good:**
  - One path for every draft, and validation can't be skipped.
  - Versions and decisions are kept.
- **Bad:**
  - A fourth API write, guarded like the others.
  - Drafts accumulate. That's a few per lead; prune discarded ones later if needed.
- **Revisit when:** drafts get edited by hand (M5), or the API gets real authentication.
