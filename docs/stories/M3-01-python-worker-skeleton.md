# M3-01: Python project and worker skeleton

- **Status:** Done
- **Milestone:** M3 — Extraction and scoring
- **Linear:** [HAM-40](https://linear.app/hamid-corp/issue/HAM-40/m3-01-python-project-and-worker-skeleton)
- **Estimate:** 1 day

## Story

As the job seeker, I want a Python worker that starts with `make up` next to the API, so
that the agents (extraction, scoring) have a tested home before any model is called.

## Context

First Python in the repo (`agents/`). CLAUDE.md fixes the toolchain: uv, ruff, pyright
strict, pytest, Pydantic for every model input and output, never `python` or `pytest`
directly. L-01 / ADR-0016 set the container rules: digest-pinned, non-root, read-only
root filesystem, ports on 127.0.0.1, config only from `.env`. The worker connects to
Postgres as the app role; the queue it will consume is M3-02.

## Acceptance criteria

1. **Given** `agents/` with `pyproject.toml` and a committed `uv.lock`
   **When** `make test-py` and `make lint` run
   **Then** pytest, ruff (lint + format check) and pyright strict all pass, and
   `make check` runs them (CI too).
2. **Given** `make up`
   **When** the stack starts
   **Then** a `worker` service starts from its own multi-stage image (Python version
   pinned, base images pinned by digest, non-root, read-only root filesystem), connects
   as `jobhunt_app`, logs one structured "worker started" line, and idles (no queue
   yet). The Compose smoke test (L-01) checks it is running.
3. **Given** SIGTERM (`make down`)
   **When** the worker is idle or in the middle of a loop iteration
   **Then** it finishes the iteration and exits 0 within 10 s (test).
4. **Given** configuration
   **When** the worker starts
   **Then** it reads only environment variables, validated by a Pydantic settings model
   once at start-up; a missing or invalid value fails fast with the variable's name (no
   value) in the error.
5. **Given** the new Python dependencies (driver, Pydantic, test tools)
   **When** they are added
   **Then** a short ADR records the Python stack (uv, psycopg 3, Pydantic, the image),
   and Dependabot watches `agents/` (`uv`).

## Out of scope

- The work queue (M3-02), any model call (M3-03), agents (M3-04+).

## Security notes

New ecosystem (PyPI): lockfile with hashes, `uv sync --frozen` in CI and in the image,
Dependabot, no install-time scripts beyond what the lock pins. Logs never contain posting
text or secrets. The worker has no published port.

## Notes for the implementer

Layout: `agents/pyproject.toml`, `agents/src/jobhunt_agents/` (`worker.py`, `settings.py`,
`db.py`), `agents/tests/`. Use psycopg 3 with a small connection pool. Keep the image's
entrypoint the worker module; no shell needed.
