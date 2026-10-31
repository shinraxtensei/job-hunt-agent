---
name: story-workflow
description: Use when starting any feature, bugfix, or change in the job-hunt-agent repo, when asked to "work on", "implement", or "pick up" a story or Linear issue, or when about to write code that has no story yet. Takes work from story to merged PR.
---

# Story workflow

Every change in this repo follows the same path. The story is the context: it tells
the implementer what to build, the reviewer what to check, and future sessions why
the code exists.

## 1. Find or write the story

- Look in `docs/stories/` for the story (files are named `M<milestone>-<nn>-<slug>.md`,
  e.g. `M1-03-lever-adapter.md`).
- If none exists, write one from `docs/stories/_template.md` **before any code**, and
  show it to the user for approval. Do not proceed on an unapproved story.
- A story is ready only when it has:
  - a user story line ("As the job seeker, I want…, so that…")
  - acceptance criteria in **Given / When / Then**, each one testable
  - an explicit **Out of scope** list
  - **Security notes** (write "none" only after actually thinking about it)
- If a story needs more than ~1 day of work or more than ~400 changed lines, split it.

## 2. Branch

`git switch -c <story-id>-<slug>`, e.g. `M1-03-lever-adapter`.

## 3. Plan

Write a short plan in the PR description draft: files to touch, tests to add, order of
work. If the plan reveals a decision (new dependency, new table, changed contract), stop
and write an ADR first (`adr` skill).

## 4. Test first, per acceptance criterion

For each Given/When/Then:
1. Write the test. Name it after the criterion.
2. Run it. **Watch it fail for the expected reason.** A test that passes immediately is
   testing nothing; fix the test.
3. Write the minimum code to pass.
4. Refactor with tests green.

Go: `go test -race ./...`. Python: `uv run pytest`. Agent behavior changes: `make evals`.

## 5. Self-review before opening the PR

Run `make check`. Then review the diff as a stranger would:
- Does every acceptance criterion have a test that would fail if it broke?
- Any ignored errors, swallowed exceptions, or `TODO` without a linked issue?
- Any untrusted posting text reaching a prompt, log, or SQL without handling?
- Any new dependency without an ADR?
- If tailoring, scoring output, or the evidence bank changed: run through the
  `evidence-bank-guardrails` checklist.

Use the review plugins when available: pr-review-toolkit, `/security-review`.

## 6. Open the PR

Title in Conventional Commit style: `feat(source): add Lever adapter (M1-03)`.

Body:
```
Story: docs/stories/M1-03-lever-adapter.md  (Linear: <link>)

## What
## How (key decisions, link ADRs)
## Acceptance criteria
- [x] AC1 … → TestLeverAdapter_ParsesPostings
- [x] AC2 … → TestLeverAdapter_RespectsRateLimit
## Security notes
## Definition of Done
- [ ] make check green   - [ ] ADR if needed   - [ ] CLAUDE.md updated if conventions changed
```

## 7. Merge and verify

Squash-merge to `main`. After deploy, confirm the behavior on the server, then mark the
story `Done` in its file and in Linear. A story is not done until it was seen working.
