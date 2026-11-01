# M0-02: CI pipeline

- **Status:** Ready
- **Milestone:** M0 — Walking skeleton
- **Linear:** [HAM-16](https://linear.app/hamid-corp/issue/HAM-16/m0-02-ci-pipeline)
- **Estimate:** ½ day

## Story

As the job seeker building this tool, I want every push and pull request checked
automatically, so that `main` is always green and deployable.

## Context

Depends on M0-01. Runs the same `make check` as local. Deployment is added in M0-05.

## Acceptance criteria

1. **Given** a pull request to `main`
   **When** it is opened or updated
   **Then** a GitHub Actions workflow runs `make check` and reports pass/fail on the PR.
2. **Given** a commit containing something that looks like a secret (API key pattern)
   **When** CI runs
   **Then** the gitleaks step fails the build.
3. **Given** the repository settings
   **When** a PR has a failing check
   **Then** it cannot be merged into `main` (branch protection requires the check).
4. **Given** Go dependencies
   **When** a new version is published
   **Then** Dependabot or Renovate opens an update PR (weekly, grouped).
5. **Given** a repeat CI run with no dependency change
   **When** it runs
   **Then** the Go module and build caches are restored (visible in the logs).

## Out of scope

- Deployment (M0-05), Python and web jobs (added with their milestones), Claude security
  review action (optional, later).

## Security notes

Workflow permissions default to `contents: read`. Third-party actions pinned by commit
SHA, not tag. No secrets needed for this story.

## Notes for the implementer

- The GitHub repo is `shinraxtensei/job-hunter` (default branch `master`); history was
  pushed with M0-01.
