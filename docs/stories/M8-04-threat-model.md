# M8-04: Threat model review

- **Status:** Done
- **Milestone:** M8 — Feedback and write-up
- **Linear:** [HAM-70](https://linear.app/hamid-corp/issue/HAM-70/m8-04-threat-model-review)
- **Estimate:** ½ day

## Story

As the job seeker about to show this project, I want the threat model checked against what
was actually built, so that every claimed control points at the code or test that enforces
it, and the gaps are written down, not discovered.

## Context

`CLAUDE.md` names `docs/threat-model.md`, but it was never written: security decisions
live in the ADRs and in each story's security notes. This story gathers them for the
finished v1 (M0–M7), on the Mac, before D puts it on a server.

## Acceptance criteria

1. **Given** the system as built
   **When** `docs/threat-model.md` is read
   **Then** it names the assets, the trust boundaries and the data that crosses each.
2. **Given** each threat
   **When** it lists a control
   **Then** the control links to the file or the test that enforces it; a control with no
   enforcement is listed as a gap, not as a control.
3. **Given** a document that disagrees with the code
   **When** the review finds it
   **Then** the finding is recorded with its fix (done here if small, else a follow-up).
4. **Given** milestone D (a server)
   **When** the model is read
   **Then** what changes on a server is listed for D's stories.

## Out of scope

- Penetration testing, and fixing large gaps (they become follow-ups).
- The git-history scan for personal data (M8-03's pre-publication step).

## Security notes

The review itself reads code and configuration only. It doesn't read `.env` values or the
personal files' content. File permissions were checked with `ls`.

## Results

`docs/threat-model.md` (2026-11-09). Findings, all handled:
- **F1:** ADR-0018 said evidence lines are never sent to the model. Since M4 the tailor,
  letter and checker calls send the selected evidence lines and the employment facts
  (employers, titles, dates, schools); never the name or contact details. ADR-0018 now
  has an amendment saying so. The owner is told, since it is data leaving the Mac.
- **F2:** `.env` and the personal profile files were world-readable (644). Now 600, and
  `data/profile` is 700. The containers still read them on the Mac.
- **F3:** `docs/threat-model.md` was referenced but missing. Now written.
- Gaps (accepted for the Mac, D items): any local process can call the API (loopback, no
  auth); backups are unencrypted (owner-only directory); the model provider keeps inputs
  up to 30 days.
