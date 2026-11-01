---
name: adr
description: Use in the job-hunt-agent repo when a decision is made or proposed - adding a dependency, adding or removing a service, changing a database table that both Go and Python use, changing a trust boundary, choosing a model provider, or reversing an earlier decision. Writes an Architecture Decision Record and updates the index in CLAUDE.md.
---

# Architecture Decision Records

An ADR records *why* the system is the way it is, so future sessions don't undo a
decision without knowing what it protected. Format: Michael Nygard's, kept short.

## When an ADR is required

- A new third-party dependency (Go module, Python package, npm package, service)
- A new process, container, or external service
- A change to a table, column, or queue payload that both Go and Python depend on
- A change to who or what is trusted (auth, network exposure, what reaches a model)
- Choosing or switching a model provider or model family
- Reversing or replacing an earlier ADR

Not needed: refactors with no behavior change, bug fixes, test additions, naming.

## How to write one

1. Find the next number: highest file in `docs/adr/` plus one, four digits.
2. Copy `docs/adr/_template.md` to `docs/adr/NNNN-kebab-title.md`.
3. Fill it in. Keep it under one page. The **Context** must state the forces honestly,
   including what we lose. The **Alternatives** section lists what was rejected and why.
4. Status starts as `Proposed`. It becomes `Accepted` when the owner approves it, in the
   same PR as the first code that relies on it.
5. Add a row to the **ADR index** table in `CLAUDE.md`.
6. If it replaces an older ADR: set the old one's status to
   `Superseded by NNNN` and link both ways. Never delete or rewrite an accepted ADR's
   decision; write a new one.

## Using ADRs while coding

- Before introducing anything an ADR rules out, read that ADR. If the context changed,
  propose a new ADR instead of quietly working around the old one.
- Reference ADRs in PR descriptions and in code comments at the place the decision bites,
  e.g. `// Postgres is the queue (ADR-0001); do not add a Redis client here.`
