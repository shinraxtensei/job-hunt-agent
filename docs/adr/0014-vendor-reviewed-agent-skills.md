# 0014. Vendor reviewed third-party agent skills, pinned by commit

- **Status:** Accepted
- **Date:** 2026-11-02
- **Story:** docs/stories/M2-06-plain-tracker-table.md

## Context

Agent skills are instructions the coding agent reads and follows, often with its full
tool access (shell, git, network, this repo's secrets in `.env`). A skill therefore
crosses a trust boundary just like a dependency does, but it is prose, so the usual
tooling (lockfiles, `govulncheck`, `npm audit`) doesn't see it. Snyk's ToxicSkills
research found prompt injection in 36% of the third-party skills it tested. M2-06 wants
four third-party design skills (impeccable, frontend-design, web-design-guidelines,
fixing-accessibility).

## Decision

Third-party skills are **vendored** into `.claude/skills/<name>/` in this repo:

- Downloaded into a temporary directory outside the repo; none of their scripts are run.
- Every file is read before anything is copied. A skill is **rejected** if it has hooks,
  executable scripts, network fetches that can't be removed, a broad `allowed-tools:
  Bash`, or instructions aimed at the agent outside the skill's stated purpose.
- Copied with its `LICENSE` and a `SOURCE.md`: repo URL, commit SHA, date, license, and
  every local change (for example, a remote rules file replaced by a vendored copy).
- Reviewed in the PR that adds it, like code.
- **Pinned** to that commit and **never auto-updated**: an update is a new PR with a diff
  against the pinned copy and a fresh review.
- **No runtime fetches:** a skill may not tell the agent to download instructions or rules
  at run time. Anything it needs lives next to it in the repo.

## Alternatives considered

- **Install from a marketplace or plugin registry:** convenient, but the content can
  change under us between sessions and nobody reviews it.
- **Reference the upstream repo at run time:** the skill's instructions would be
  whatever upstream serves that day.
- **Don't use third-party skills:** safest, but these encode useful design and
  accessibility practice that would otherwise be rewritten by hand.

## Consequences

- **Good:** every instruction the agent follows from a skill is in git, reviewed, and
  diffable; a compromised upstream can't reach this repo without a PR.
- **Bad:** updates are manual and we may lag upstream fixes; vendored skills add files
  to review.
- **Revisit when:** skills get signed releases or an integrity-pinned install mechanism.
