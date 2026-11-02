# Source: web-design-guidelines

| | |
|---|---|
| Repository (skill) | https://github.com/vercel-labs/agent-skills (path `skills/web-design-guidelines`) |
| Commit (skill) | `1a2c79d4e6c4a2283dc2c614b6538cbe1ed5ca62` (2026-10-27) |
| Repository (rules) | https://github.com/vercel-labs/web-interface-guidelines (file `command.md`) |
| Commit (rules) | `434b7f91364665f2f733b310ec54809bf8f37937` (2026-10-05) |
| Vendored | 2026-11-02 |
| License | MIT. `LICENSE` is web-interface-guidelines' (Copyright 2025 Vercel Labs). agent-skills has no LICENSE file; its README and GitHub metadata say MIT. |

Vendored under ADR-0014: downloaded outside the repo, every file read, no scripts run.
Pinned to the commit below and never auto-updated; an update is a new PR with a diff
against this copy and a fresh review.

**Review:** the upstream `SKILL.md` told the agent to fetch the rules with WebFetch from
`raw.githubusercontent.com/.../main/command.md` before every review, i.e. instructions
from the network at run time (rejected by ADR-0014). The rules file itself is a plain
checklist with an output format; no scripts, hooks or `allowed-tools`. The upstream
`web-design-guidelines.zip` (a copy of the same SKILL.md) was not vendored.

**Local changes:**
- `rules.md` is upstream `command.md` at the rules commit, unchanged.
- `SKILL.md`: the "fetch the latest guidelines" steps and the URL now read the local
  `rules.md`, and say not to fetch from the network. Nothing else changed.

**Note:** rules.md asks for Title Case headings; frontend-design asks for sentence case.
This project follows sentence case (frontend-design wins on copy).
