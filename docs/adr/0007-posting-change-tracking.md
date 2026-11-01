# 0007. Track posting changes with a content hash; keep the latest payload

- **Status:** Accepted
- **Date:** 2026-11-01
- **Story:** docs/stories/M1-07-idempotent-posting-upsert.md

## Context

Since M0-04 a re-seen posting only moves `last_checked_at`; the payload from the first
sighting is kept forever, so an edited posting (new title, salary, description) is never
seen. `postings` is the contract the Python agents read, and they will re-extract and
re-score a posting when it changes, so "did it change, and when" has to be cheap to ask.
Payloads are ~5–15 KB and the collector re-sees thousands every run.

## Decision

- `postings.content_hash bytea NOT NULL`: SHA-256 of the posting's canonical `jsonb` text
  (`sha256(convert_to(raw_payload::text, 'UTF8'))`), computed **in Postgres** in the upsert,
  so new and stored rows are hashed the same way whatever the source's key order or
  whitespace.
- `postings.last_changed_at timestamptz NOT NULL`: when the payload last changed; equal to
  `first_seen_at` until the first change. Existing rows are backfilled that way.
- On conflict the upsert writes the new payload, hash, URL and title; `last_changed_at`
  moves only when the hash differs. It reports `inserted` and `changed`; the collector
  counts new / changed / seen (seen = identical payload, only `last_checked_at` moved) and
  records `collector_runs.postings_changed` next to `postings_seen` (ADR-0006's "add
  columns").
- **Only the latest payload is kept.** History is the three timestamps, not old versions.

## Alternatives considered

- **Compare `raw_payload` directly** (`IS DISTINCT FROM`): works, but every consumer that
  wants "changed since" pays for jsonb comparison, and Python can't compare cheaply
  without the hash.
- **Hash in Go:** the source's byte order would differ from Postgres's canonical jsonb text
  and from what Python sees; one canonical place avoids false changes.
- **A `posting_versions` table** keeping every payload: real history, but storage grows
  with every edit and nothing needs old versions yet.
- **A generated column:** `convert_to` isn't immutable, so Postgres won't allow it.

## Consequences

- **Good:** "did it change" is one hash comparison; reruns stay idempotent;
  the stored payload is always current, so re-extraction uses what the board shows today.
- **Bad:** an edited posting's previous text is gone (so M6 stores a snapshot of the posting
  on each application, see the roadmap); a source that adds a volatile field
  (a timestamp on every response) would mark everything changed on every run.
- **Revisit when:** we need to show what changed in a posting, or a source turns out to
  have volatile fields (then hash a normalised subset for that source).
