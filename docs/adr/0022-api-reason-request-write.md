# 0022. The API's first write: request a fresh reason, refused cross-site; loopback Host names only

- **Status:** Accepted
- **Date:** 2026-11-06
- **Story:** docs/stories/M3-08-score-in-the-tracker.md

## Context

M3-06 (ADR-0021) regenerates a lead's reason lazily: only when the lead is opened and
its reason was made from older inputs. Opening happens in the tracker page, which so far
reads a read-only API (ADR-0012).

So the page needs a way to say "this lead was opened". Each request can spend model
budget, about $0.0002 a lead, capped by the daily budget.

The API has no authentication (ADR-0002, Proposed); access is limited by the network. A
page on another site that the owner visits could still send a request to the tracker's
address from the owner's browser.

## Decision

- **`POST /api/leads/{id}/reason`** sets `scores.reason_requested_at = now()`, and only
  when the lead's reason is not current. The worker queues the regeneration on its next
  iteration (ADR-0021).
  - The answer is `202 {"requested": true|false}`.
  - Nothing else in the database changes, and the request has no body.
- **Cross-site requests are refused (403)** by Go's `http.CrossOriginProtection`, and by a
  content-type check:
  - `CrossOriginProtection` reads `Sec-Fetch-Site`. Without that header it compares
    `Origin` with `Host`. It lets same-origin requests and non-browser ones (neither
    header) through.
  - The `Content-Type` must be `application/json`. A JSON content type makes the
    browser send a CORS preflight, which the API never answers.
- **Only loopback Host names are served, on every route, reads and writes (review of
  #43).** The API has no auth and relies on being reachable on loopback only. A page
  whose domain is rebound to 127.0.0.1 (DNS rebinding) is same-origin with itself, so
  the cross-origin checks above don't stop it; its requests carry its own name as `Host`.
  - `HostAllowlist` (wrapping the whole server in `cmd/api`) serves only `127.0.0.1`,
    `localhost` or `[::1]`, and only on the listen port or `API_PUBLIC_PORT` (the port
    Compose publishes).
  - Anything else gets 421 Misdirected Request.
  - The Vite dev proxy rewrites `Host` to the API's own (`changeOrigin`).
- **All other endpoints stay read-only.** The app role already has UPDATE on `scores`
  (the worker writes it).

## Alternatives considered

- **The read endpoint (GET) marks the lead requested:** a GET with a side effect. Link
  prefetching or a cross-site image tag would trigger it.
- **Regenerate every stale reason in the background:** simpler, but it spends model
  budget on leads the owner never opens. That is what ADR-0021's lazy rule avoids.
- **A CSRF token:** needs state or a secret for one timestamp write. The fetch-metadata
  and content-type checks are enough while the API has no sessions.

## Consequences

- **Good:** opening a lead is the only thing that spends money on its reason. A foreign
  page can't trigger it.
- **Bad:**
  - The API is no longer read-only, so its tests now include a write.
  - Clients without `Sec-Fetch-Site`, such as curl, are allowed when they send a JSON
    content type. That's fine on the owner's own network.
- **Revisit when:** the API gets real authentication (ADR-0002, decided in D, or S for
  sharing). Then the write moves behind the session.
  - At D, the server's Tailscale name must join the Host allowlist: config, not code.
