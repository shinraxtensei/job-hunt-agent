# 0016. One distroless image and a Compose stack, the same on the Mac and the server

- **Status:** Accepted
- **Date:** 2026-11-03
- **Story:** docs/stories/L-01-whole-stack-on-the-mac.md

## Context

The roadmap now runs the whole project on the owner's Mac first (milestone L); the server
(D) comes last. Until now only Postgres ran in Compose; the API and migrations ran with
`go run`. Whatever runs locally should be what D deploys, so the server step is moving
images, not rebuilding the setup. The API has no auth (ADR-0002).

## Decision

- **One image, `jobhunt`,** from a multi-stage `Dockerfile` at the repository root:
  Node builds the tracker page (ADR-0013), Go builds static binaries (`api`,
  `collector`, `migrate`, `CGO_ENABLED=0`, `-trimpath`, the commit from `GIT_SHA`), and
  the runtime stage is `gcr.io/distroless/static-debian12:nonroot`: no shell, no package
  manager, user `nonroot`. Every base image (Node, Go, distroless, Postgres) is pinned by
  digest; Dependabot proposes bumps for the Dockerfile and the Compose file.
- **`deploy/compose.yaml`:** `postgres`, `migrate` (runs once, the API waits for it to
  succeed), `api` (serves the API and the page, healthchecked by `api healthcheck`, since
  the image has no curl) and `collector` (profile `tools`: only `make collect` runs it).
  App containers: read-only root filesystem, a `tmpfs` `/tmp`, all capabilities dropped,
  `no-new-privileges`. Postgres: read-only root and `no-new-privileges`, but it keeps its
  capabilities: the official entrypoint starts as root to prepare its data directory, then
  drops to `postgres`.
- **Ports only on 127.0.0.1** (`JOBHUNT_HTTP_PORT`, default 8080; `JOBHUNT_DB_PORT`,
  default 5432). Inside its container the API listens on all interfaces (it must, to be
  reachable through the published port) and says so in its log instead of warning.
- **Config only from the git-ignored `.env`;** the stack builds its connection URLs from
  the passwords there.
- **Backups:** `make backup` runs `pg_dump --format=custom` in the Postgres container into
  `JOBHUNT_BACKUP_DIR` (default `~/job-hunter-backups`, outside the repository), mode 600
  in a 700 folder, keeping the newest 14; `make restore FILE=…` stops the API, restores in
  one transaction with `--clean --if-exists`, and starts it again.
- **CI** runs `deploy/scripts/smoke.sh`: build, up, `/healthz`, `/api/leads`, the page,
  backup rotation, a backup restored, down, in its own project with its own ports.

## Alternatives considered

- **One image per binary:** three builds of the same code for no isolation gain; one image
  with three entrypoints is simpler to pin and to move to the server.
- **Alpine or Debian runtime:** a shell and a package manager in production for nothing the
  binaries need; distroless is smaller and has less to patch.
- **Running binaries on the Mac with `go run` (as before):** works, but isn't what the server
  will run, and leaves migrations and start order to the owner.
- **Postgres fully non-root (`user:` set):** needs a data directory prepared outside the
  image's entrypoint; not worth it on a loopback-only local stack. Revisit in D.

## Consequences

- **Good:** `make up` is the whole project; the server step (D) reuses the images and the
  Compose file; containers can write only to their volume and `/tmp`.
- **Bad:** a Docker build on every `make up` with code changes (cached layers keep it to
  seconds after the first); no shell in the image for debugging (use `docker compose exec
  postgres psql` or a debug image).
- **Revisit when:** D (the server, TLS behind Caddy, Tailscale), or when the M3 worker
  (Python) needs its own image.
- **Note (2026-11-06, ADR-0022):** the API serves only loopback `Host` names on its
  listen port and `API_PUBLIC_PORT` (Compose sets it to `JOBHUNT_HTTP_PORT`), against
  DNS rebinding. At D, behind Caddy and Tailscale, the served name changes. Add it to
  the allowlist through configuration, and decide whether Caddy or the API checks it.
