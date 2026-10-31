# 0002. No built-in auth; restrict access with Tailscale

- **Status:** Proposed
- **Date:** 2026-10-31
- **Story:** Week 0 — walking skeleton

## Context

Version 1 has exactly one user, its owner. The data is personal: CV, application history,
contacts. Writing authentication (password hashing, sessions, CSRF, reset flows, rate
limiting) is a large surface to get wrong, and none of it is the point of this project.
The UI must still be reachable from the owner's phone.

## Decision

The web UI and API are not exposed to the public internet. The server joins the owner's
Tailscale network; Caddy listens only on the Tailscale interface. The app has no login.
The VPS firewall allows only SSH (key-only) and Tailscale.

## Alternatives considered

- **Build session auth:** real security work for no user-facing benefit in v1.
- **Cloudflare Access:** also good; rejected only to avoid routing personal data through
  a third-party proxy and to keep one networking tool.
- **HTTP basic auth on a public port:** weak against brute force and leaks the app's
  existence.

## Consequences

- **Good:** zero auth code; nothing public to attack except SSH; works on phone and laptop.
- **Bad:** every device needs Tailscale installed; opening to other users later requires
  real auth (a new ADR).
- **Revisit when:** a second user is added, or the PRD's open question about opening the
  tool to others is answered yes.
