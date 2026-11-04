# 0017. Python 3.15 with uv, psycopg 3 and Pydantic, in a slim image pinned by digest

- **Status:** Accepted
- **Date:** 2026-11-04
- **Story:** docs/stories/M3-01-python-worker-skeleton.md

## Context

M3 adds the first Python: one worker that will run extraction and scoring (CLAUDE.md:
Go does network work, Python does model work, Postgres is the contract). CLAUDE.md
already fixes uv, ruff, pyright strict, pytest and Pydantic; what is left to decide is
the Python version, the database driver, how settings are loaded, and the image. The
image must meet ADR-0016's rules: pinned by digest, non-root, read-only root, all
capabilities dropped, `no-new-privileges`, no published port.

## Decision

- **Python 3.15.2**, pinned in `agents/.python-version` and `requires-python = "==3.15.*"`.
  Checked on 2026-11-04, not assumed: uv 0.13.7 installs it; psycopg-binary 3.3.8,
  pydantic-core 2.50.2 (pydantic 2.14.2) publish cp315 wheels for macOS arm64 and Linux
  x86_64/arm64; pyright 1.1.414 accepts `pythonVersion = "3.15"`; ruff 0.17.3 has
  `py315`.
- **uv** manages the interpreter, the virtual environment and `uv.lock` (with hashes).
  CI and the image install with `uv sync --frozen`: the lock is never re-resolved.
- **psycopg 3** (`psycopg[binary,pool]`): the maintained Postgres driver, typed, with a
  connection pool (`psycopg_pool`). The worker opens a small pool (`min 1`, `max
  WORKER_POOL_MAX`, default 2) as `jobhunt_app`. SQL stays parameterised; no ORM.
- **pydantic-settings** reads configuration from environment variables once at start-up.
  Secrets are `SecretStr`. A missing or invalid variable fails fast with its name; the
  error is built from the validation error's locations only, never its input.
- **Dev tools** (dependency group `dev`, pinned exactly): ruff, pyright, pytest,
  testcontainers (a real Postgres from `deploy/compose.yaml`'s image, like the Go tests).
- **Image: `python:3.15.2-slim-trixie` pinned by digest**, built by `agents/Dockerfile`:
  a builder stage copies the pinned uv binary (`ghcr.io/astral-sh/uv`, by digest) and
  runs `uv sync --frozen --no-dev --no-editable`; the runtime stage copies only the
  virtual environment, runs as a fixed non-root user (uid 10001), and its entrypoint is
  `python -m jobhunt_agents`. Compose runs it with ADR-0016's hardening and no ports; its
  healthcheck is `python -m jobhunt_agents healthcheck` (a heartbeat file in `/tmp`).
- **Graceful shutdown:** SIGTERM and SIGINT set a stop flag; the loop finishes the
  iteration in progress, closes the pool and exits 0 (Compose waits 10 s).
- **Dependabot** watches `agents/` (`uv`) and `agents/Dockerfile` (`docker`).

## Alternatives considered

- **`gcr.io/distroless/python3-debian13`:** no shell, smaller attack surface, but its
  Python is Debian's, 3.13.5 on trixie: the interpreter would be chosen by the Debian
  release, not by us, and would differ from the one tested on the Mac and in CI. Slim
  gives an exact, tested version; the cost is a shell and apt in the image (mitigated
  by non-root, read-only root, no capabilities, no port).
- **Python 3.14:** also fully supported, but nothing needs it over 3.15, and starting on
  the latest stable line postpones the first upgrade.
- **asyncpg / async worker:** faster for many concurrent queries, but the worker does one
  thing at a time (it waits on the model, not on Postgres) and psycopg 3 is typed and has
  a pool. Revisit only if the worker needs many concurrent calls.
- **python-dotenv / hand-written `os.environ` parsing:** pydantic-settings is the same
  Pydantic the agents already use, with types and validation for free.
- **pip + requirements.txt:** no lockfile with hashes for the whole graph, no managed
  interpreter; CLAUDE.md already chose uv.

## Consequences

- **Good:** one toolchain (uv) on the Mac, in CI and in the image; the same interpreter
  everywhere; typed settings that fail fast and never print secrets.
- **Bad:** a second base image to keep current (Dependabot); the slim image has a shell,
  which distroless would not; Python 3.15 is new, so a library we add later may lack
  wheels (check before adding).
- **Revisit when:** distroless ships the Python we use, or a dependency forces another
  version.
