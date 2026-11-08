# M6-06: Company merge

- **Status:** Done
- **Milestone:** M6 — Full tracker
- **Linear:** [HAM-64](https://linear.app/hamid-corp/issue/HAM-64/m6-06-company-merge)
- **Estimate:** ½ day

## Story

As the job seeker, I want to merge a company I added by hand with the same company on
the watch list, so that their leads group together and nothing is duplicated.

## Context

From M2-07 (ADR-0015): a board-less company typed with `collector add --company` and
the same company later added to the watch list are two `companies` rows. The roadmap
puts the merge in M6. A CLI command, like the other collector maintenance commands.

## Acceptance criteria

1. **Given** two companies
   **When** I run `collector merge-company <from> <into> [--dry-run]`
   **Then** it prints what moves (postings, jobs, applications), and without
   `--dry-run` moves them to `<into>` in one transaction, recomputes duplicate groups,
   and marks `<from>` inactive (never deleted).
2. **Given** `<from>` is on the watch list (YAML)
   **When** the merge runs
   **Then** it refuses: only a manual (board-less) company can be merged away.
3. **Given** an application on a lead of `<from>`
   **When** the merge runs
   **Then** the application, its history and its documents stay attached.

## Results (2026-11-08)

- **Command:** `collector merge-company <from> <into> [--dry-run]` takes ids or names. A
  name prefers the hand-added company for `<from>` and the watch-list one for `<into>`;
  when a name is ambiguous, the command asks for the id. It prints the plan, then moves
  postings, jobs and applications in one transaction. Groups follow, since `job_groups`
  is a view.
- **Refusals (exit 2, nothing changed):** a watch-list company as `<from>`, a merge into
  itself, an unknown company, or both companies holding an application for the same role.
- **AC1 change:** the step that marked `<from>` inactive was removed. Hand-added
  companies are always inactive (ADR-0015), so it did nothing; the mutation check showed
  it. The company is kept, never deleted.
- **Tests:** `TestMergeCompany`. Each check was broken on purpose and its test seen to
  fail.

## Out of scope

- Merging two watch-list companies. Automatic detection.

## Security notes

- A local CLI on the owner's database; no network.
