# M4-01: Evidence bank

- **Status:** Ready
- **Milestone:** M4 — Evidence bank and tailoring
- **Linear:** [HAM-49](https://linear.app/hamid-corp/issue/HAM-49/m4-01-evidence-bank)
- **Estimate:** 1 day

## Story

As the job seeker, I want my Master CV kept as tagged, checked evidence lines, so that
every tailored CV can only say what I have actually done, in words I approved.

## Context

PRD: "tagged Master CV (BE, PAY, DIST, FS, AI, CONS) → evidence bank"; guardrail rule 1
("the evidence bank is the only source"). Builds on the skill map (M3-05). M4-02
(tailoring), M4-03 (coverage) and M4-04 (validator) read it. It is personal data, kept
like the skill map (ADR-0019): a git-ignored file read by the worker, not edited in a
UI in this story.

## Owner input

**OI-1** (Master CV as `data/profile/evidence.yaml`) and optionally **OI-2**; formats in
`docs/stories/M4-owner-input.md`. This story ships the loader with the committed
`evidence.example.yaml`. It can be built and tested before OI-1 arrives. It is
seen working once OI-1 imports cleanly.

## Acceptance criteria

1. **Given** `data/profile/evidence.yaml`
   **When** the worker loads it
   **Then** it is validated before anything uses it, and every problem is reported with
   its line number:
   - ids are unique;
   - each `skills` name resolves in the skill map (aliases included);
   - dates are `YYYY-MM` or `present`;
   - every line has `checked: true`;
   - variants are among BE PAY DIST FS AI CONS;
   - a `learning` skill may be named only in a **project** line, stated as a fact
     ("Built a job collector in Go"). In a role line it is refused. Roles, the summary
     and the skills section never present learning as experience.
   A broken file keeps the last good version (as the profile does). Table tests use the
   example file plus one broken file per rule.
2. **Given** a valid file
   **When** it is loaded
   **Then** it is stored in Postgres with the file's hash as its version, so the API
   (M4-05) and drafts can cite lines by id. Roles, projects, education and lines are
   replaced in one transaction. A line removed from the file is kept for drafts that
   already cite it, marked `retired` (integration test).
3. **Given** each evidence line
   **When** it is stored
   **Then** code records, for later checks (M4-04):
   - the numbers it contains, normalised ("~3 years", "40%", "1.2k");
   - its skills, with their levels from the skill map.
   Tests cover each number form.
4. **Given** the files are personal
   **When** the repository is shared (M8)
   **Then** `evidence.yaml` and `master_cv.*` are git-ignored, and the committed
   `evidence.example.yaml` shows the format with placeholder data. `make backup`
   includes them; logs never contain line text.

## Out of scope

- Editing evidence in the UI (a later story). Importing a CV document automatically:
  if the owner chooses that path in OI-1, Claude drafts the file once, outside the
  product, and every line stays `checked: false` until the owner checks it.
- Tailoring (M4-02), coverage (M4-03).

## Security notes

- Personal data: the file, the tables and backups stay on the owner's machine. Logs
  carry ids and counts only.
- No model sees the whole bank. M4-02 sends only the lines selected for one lead
  (guardrail "Generation").
- The DB tables are written only by the worker (app role).
- ADR: new shared tables read by the API (`evidence_*`), so an ADR is required.
