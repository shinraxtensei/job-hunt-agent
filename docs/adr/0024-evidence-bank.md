# 0024. The evidence bank: a checked YAML file, synced by the worker into versioned tables

- **Status:** Accepted
- **Date:** 2026-11-06
- **Story:** docs/stories/M4-01-evidence-bank.md

## Context

Tailored CVs may say only what is in the owner's evidence (guardrail rule 1). Several
parts read that evidence:
- the tailoring agent (Python, M4-02);
- the coverage report and the validator (Python, M4-03/M4-04);
- the draft review page (Go API and web, M4-05).

Drafts cite evidence lines by id. A cited line must still be readable after the owner
edits or removes it, or an old draft can't be checked.

The owner edits evidence as a file, like the skill map (ADR-0019): reviewed, diffable,
backed up, and never committed (personal data).

## Decision

- **`data/profile/evidence.yaml`** is the source.
  - The worker reads it with ruamel.yaml and validates it with line numbers, like the
    profile.
  - A broken edit keeps the last good version and is logged.
  - Every line must be `checked: true`. A drafted line is `checked: false` until the
    owner confirms it.
- **The worker syncs it into Postgres** when its version changes. The version is the
  sha256 of the file plus the skill map's version, because skill levels are recorded
  per line.
  - **`evidence_items`:** the person (header), summary, roles, projects, education and
    certifications. Each holds its exact facts as `jsonb`.
  - **`evidence_lines`:** one row per line, with:
    - its parent item, text, variants and `source`;
    - its skills, each with the level it had;
    - its normalised numbers.
  - **`evidence_bank`:** one row with the current version and when it was loaded.
  - The sync runs in one transaction. Rows are upserted by id. Ids missing from the
    file are marked `retired`, never deleted, so drafts that cite them stay checkable.
- **Learning skills** may be named only in project lines (owner's review of #45).
- **Numbers** are extracted in code (`numbers.py`). Digits that are part of a name
  (OAuth2, S3, GPT-4, K8s, Python 3) are not numbers. M4-04 uses the same function.
- **No model** reads the whole bank. M4-02 sends the lines it selects for one lead.

## Alternatives considered

- **Edit evidence in the UI and keep it only in Postgres:** no diff, no review, and a
  second copy of personal data to back up differently. A UI editor can come later and
  write the same file.
- **Keep it only in the file and read it on each request:** the Go API would need its
  own YAML parser and validation, and drafts couldn't cite retired lines.
- **Delete removed lines:** old drafts would cite ids that no longer exist.

## Consequences

- **Good:**
  - One reviewed source, with drafts citing stable ids.
  - The API reads plain tables.
  - Old drafts stay checkable.
- **Bad:**
  - Retired rows accumulate. That's a few hundred at most; prune when a draft no
    longer cites them.
  - The DB copy is as fresh as the worker's last iteration (seconds).
- **Revisit when:** evidence gets a UI editor, or several people's banks (S: sharing).
