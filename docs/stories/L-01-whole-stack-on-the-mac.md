# L-01: Whole stack in Docker Compose on the Mac

- **Status:** Done
- **Milestone:** L — Whole stack on the Mac
- **Linear:** [HAM-39](https://linear.app/hamid-corp/issue/HAM-39/l-01-whole-stack-in-docker-compose-on-the-mac)
- **Estimate:** 1 day

## Story

As the job seeker, I want the whole project to run on my Mac with one command, the same
way it will later run on a server, so that I use the tracker every day without a server
and my data is safe.

## Context

Roadmap change (2026-11-03): the server (D) comes last. Until then everything runs on
the Mac, collection runs on demand, and backups are local. Today `make up` starts only
Postgres; the API and migrations run with `go run`. The worker arrives in M3-01.

## Acceptance criteria

1. **Given** a `.env` made from `.env.example`
   **When** I run `make up`
   **Then** Postgres starts, migrations run, and the API starts serving the tracker; one
   command, then http://127.0.0.1:8080 works. Every published port is bound to
   127.0.0.1 only. `make down` stops everything and the data stays in a named volume.
2. **Given** the stack is up
   **When** I run `make collect`
   **Then** `collector run` runs inside the stack (on demand, no schedule) and its run is
   recorded like any other.
3. **Given** the images
   **When** they are built
   **Then** they are multi-stage, every base image is pinned by digest, they run as a
   non-root user, with a read-only root filesystem where possible: the same images the
   server will use (D).
4. **Given** `make backup`
   **When** it runs
   **Then** it writes a timestamped `pg_dump` (custom format) to a folder outside the repo
   (`JOBHUNT_BACKUP_DIR`, default `~/job-hunter-backups`), readable only by the owner,
   and keeps the last 14. `make restore FILE=…` restores one into the stack. A restore
   was tested once for real.
5. **Given** secrets and config
   **When** the stack runs
   **Then** they come only from the git-ignored `.env`; `.env.example` is committed.
6. **Given** CI
   **When** it runs
   **Then** a Compose smoke test builds the images, starts the stack, waits for
   `/healthz`, gets `/api/leads`, and stops it.

## Out of scope

- The worker service (M3-01). A daily schedule, alerting, off-machine backups (D).

## Security notes

Ports on 127.0.0.1 only (the API has no auth, ADR-0002). Containers run non-root, with
no capabilities and no privilege escalation. Backups hold all posting data: written
with mode 600 into a 700 folder outside the repository.
