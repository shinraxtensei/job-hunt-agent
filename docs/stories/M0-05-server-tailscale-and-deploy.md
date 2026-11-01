# M0-05: Server, Tailscale and deploy

- **Status:** Ready
- **Milestone:** M0 — Walking skeleton
- **Linear:** —
- **Estimate:** 1 day

## Story

As the job seeker, I want every merge to `main` deployed to my own server that only I can
reach, so that the tool is real from week one and shipping is never a big event.

## Context

ADR-0002 (Tailscale instead of auth). One small VPS running Docker Compose. Images built
in CI and pushed to GHCR.

## Acceptance criteria

1. **Given** a fresh VPS
   **When** the documented setup in `deploy/README.md` is followed
   **Then** the server has Docker, joins the tailnet, allows only SSH (keys only) and
   Tailscale through its firewall, and has automatic security updates enabled.
2. **Given** a merge to `main` with green CI
   **When** the deploy workflow runs
   **Then** it builds and pushes images tagged with the commit SHA, runs migrations, then
   restarts services with `docker compose pull && docker compose up -d`.
3. **Given** a failed migration
   **When** the deploy runs
   **Then** the services are not restarted and the workflow fails visibly.
4. **Given** the deployed stack
   **When** I open `https://<server>.<tailnet>.ts.net/healthz` from my phone on Tailscale
   **Then** it returns 200 with the commit SHA; from outside the tailnet it is unreachable.
5. **Given** the server
   **When** I run the collector container once with a Lever company set
   **Then** postings appear in the server's database (M0-04 proven in production).

## Out of scope

- Scheduled runs (M1-08), backups (M1-09), the web UI (M2-06).

## Security notes

Secrets (DB password, Tailscale auth key, SSH deploy key) live only in GitHub Actions
secrets and a root-only `.env` on the server. The deploy SSH key is restricted to the
deploy command. Postgres is not exposed on any interface outside the compose network.
