# M8-03: Public README and architecture write-up

- **Status:** In progress
- **Milestone:** M8 — Feedback and write-up
- **Linear:** [HAM-71](https://linear.app/hamid-corp/issue/HAM-71/m8-03-public-readme-and-architecture-write-up)
- **Estimate:** 1 day

## Story

As the job seeker showing this project, I want a README that explains what it does, how
it is built and why, with screenshots, so that a reader understands it in five minutes,
without any of my personal data being shown.

## Context

The README today is the owner's daily guide. A public reader also needs the idea, the
architecture, the honesty guardrails, the decisions (ADRs), the numbers (M8-02) and the
security model (M8-04). Screenshots must never show the owner's data (owner, 2026-11-09).

## Acceptance criteria

1. **Given** the README
   **When** a stranger reads it
   **Then** it has: what it does and doesn't, screenshots, an architecture diagram, the
   honesty guardrails, the ADR index, how to use it daily, benchmarks and security links,
   and how to run it.
2. **Given** the screenshots
   **When** they are made
   **Then** they come only from a committed demo data set (the example profile "Sam
   Example" and invented leads), served to the built page through Playwright mocks: no
   database and no API is reachable while they're taken.
3. **Given** the architecture diagram
   **When** it renders on GitHub
   **Then** it is Mermaid text in the README (no image to keep in sync), matching the
   ADRs: Go collector and API, Python worker, Postgres as the only shared state.
4. **Given** the git history
   **When** the pre-publication scan runs (gitleaks and a custom check for the owner's
   personal data)
   **Then** every finding is reported with its commit and path and an exact rewrite plan.
   Nothing is rewritten and the repository's visibility doesn't change: both are the
   owner's to approve.

## Out of scope

- Rewriting history, making the repository public, choosing a licence (owner decisions).

## Security notes

- Screenshots: demo data only; the script blocks every request that isn't the built page
  or a mocked `/api` path.
- The history scan reads the history locally; its report names paths and commits, and
  quotes no personal values (they are masked).

## Results (2026-11-09)

- **README** rewritten for a public reader: what it does, demo screenshots, a Mermaid
  architecture diagram, the honesty guardrails, how it was built with an ADR selection,
  benchmarks and threat-model links, how to run it and use it daily (updated for the M7
  pill navigation and tabs).
- **Demo data:** `docs/demo/demo.mjs` (Sam Example and 10 invented leads) and
  `docs/demo/shoot.mjs`: every non-page request is aborted, every `/api` path answered
  from the demo set. Seven screenshots in `docs/screens/`.
- **History scan:** gitleaks found nothing; `agents/tools/history_scan.py` (`make
  history-scan`) found no evidence text, contact detail, `.env` secret or real reason in
  any text blob. Findings H1–H5 and the rewrite plan are in `docs/publication-plan.md`.
  Real-data screenshots and the M2-06 mockups are removed from the tree; the history
  still has them until the owner runs or approves the plan.
