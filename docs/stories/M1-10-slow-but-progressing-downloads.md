# M1-10: Slow but progressing downloads

- **Status:** Done
- **Milestone:** M1 — Collector
- **Linear:** [HAM-29](https://linear.app/hamid-corp/issue/HAM-29/m1-10-slow-but-progressing-downloads)
- **Estimate:** ½ day

## Story

As the job seeker, I want a board that answers slowly but keeps sending to still be
collected, so that a slow day at one ATS doesn't silently drop its companies from my
leads, while a dead or stuck server still fails quickly.

## Context

`fetch.Client` (shared by every adapter since M1-04) uses `http.Client.Timeout = 30s`,
which caps the **whole** transfer: connecting, waiting for headers and reading the body.
On 2026-11-01 Lever answered within ~3s but then sent at ~12 KB/s, so every Lever board
(Toptal's is 1.27 MB) hit the 30s cap while still making progress. All four Lever companies
failed three times in a row (runs 5 and 6 ended `partial`), found while testing M1-07.

**Re-checked before building (2026-11-01 18:19 UTC):** the slowness persisted. One
`curl` of `api.lever.co/v0/postings/toptal?mode=json` returned 1,266,234 bytes in 109.6s
at 11.5 KB/s (first byte after 2.1s).

Replace the single total cap with timeouts that each catch one failure mode:

- **Dial, TLS and response-header timeouts:** a server that is dead or never answers fails
  fast.
- **A stall timeout:** while reading the body, fail if no bytes arrive for N seconds. A
  stalled transfer is retried, like any transient failure.
- **A generous overall ceiling** per request, so a pathological trickle still ends. Hitting
  it is not retried: a second attempt would trickle the same way.

Sizing, from the stored payloads (2026-11-01): the largest Lever board (Theodo) is ~3.3 MB,
about 290s at 11.5 KB/s; the largest board on any host (Airwallex, Ashby) is ~8 MB, about
710s at that speed. A 5-minute ceiling would cut Theodo off at today's speed, so the default
is **10 minutes**.

## Acceptance criteria

1. **Given** a server that sends a body steadily but slowly (longer in total than the old
   30s-style cap, never pausing longer than the stall timeout)
   **When** it is fetched
   **Then** the fetch completes with the full body.
2. **Given** a server that stops sending partway through the body
   **When** it is fetched
   **Then** the attempt fails after the stall timeout with an error saying it stalled, and is
   retried (at most 3 attempts in all).
3. **Given** a server that accepts the connection but never sends response headers
   **When** it is fetched
   **Then** each attempt fails after the response-header timeout, well before the stall
   timeout or the ceiling.
4. **Given** a server that keeps trickling bytes (never stalling) past the overall ceiling
   **When** it is fetched
   **Then** the fetch fails when the ceiling is reached, with an error saying so, and is not
   retried.
5. **Given** retries caused by any of the above
   **When** they happen
   **Then** every attempt still waits on the host's limiter first.

All tested with `httptest` servers that control their own write pacing; no real Lever in
tests.

## Out of scope

- Per-source timeout tuning (one set of defaults for every adapter).
- Resuming partial downloads (`Range` requests).

## Security notes

Untrusted input: response bodies are still capped at 32 MiB; the ceiling also bounds how
long an untrusted server can hold a connection. No new dependency (standard library
`net/http`, `net`, `time`). Errors still carry status or timing only, never the body.

## Notes for the implementer

- All in `internal/source/fetch`; adapters don't change.
- `http.Client.Timeout` goes to zero; the dial/TLS/header timeouts live on the
  `http.Transport`; the ceiling is a per-attempt `context.WithTimeout`; the stall timeout is
  a timer reset on every successful body read that cancels the attempt's context.
