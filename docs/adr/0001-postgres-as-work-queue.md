# 0001. Use PostgreSQL as the work queue instead of Redis

- **Status:** Accepted (revised 2026-11-04 in M3-02: leases and fencing, enqueue by reconcile)
- **Date:** 2026-10-31
- **Story:** Week 0 — walking skeleton; built in docs/stories/M3-02-work-queue.md

## Context

The PRD hands postings from the Go pipeline to the Python agents through a Redis queue.
This is a single-user system running on one small server, built and operated by one
person. Every extra service has to be deployed, monitored, backed up, and understood
when it fails at 2 a.m. PostgreSQL is already required for all records, and the owner
knows it well.

Volume is small: hundreds to low thousands of postings per day. Model calls are made
only for the leads the `default` preset shows (~500), and a call can take tens of
seconds.

## Decision

A `work_items` table in PostgreSQL is the queue. Its schema is the contract between Go
and Python, listed in `db/contract/work_items.columns`, and changed only through a
migration plus an ADR. One contract test per side compares the migrated table to that
file.

- **One row per posting and kind** (`UNIQUE (posting_id, kind)`), holding the
  `content_hash` it is for. A `done` row is the record that this content was processed;
  the table stays as small as the set of shown postings (no archive needed).
- **Go enqueues by reconciling,** not in the transaction that writes the posting:
  whether a posting needs work depends on the preset, which is evaluated in Go over
  duplicate groups (ADR-0011, ADR-0012). The reconcile step is "an `extract` item for
  every group the default preset shows whose canonical posting has no item for its
  current `content_hash`": an upsert (`ON CONFLICT (posting_id, kind) DO UPDATE … WHERE
  the hash differs`) in batches of 500, so it is idempotent and safe to run from two
  places at once. It runs after `collector run`, after `collector add`, before the
  worker starts (a one-shot `reconcile` Compose service the worker depends on), and on
  demand (`collector enqueue`, after a preset change). A re-queued item gets a new
  state, zero attempts and no claim token.
- **Lease, not a long transaction.** A worker claims in one short statement and commits:
  `UPDATE work_items SET state = 'running', locked_until = now() + lease, attempts =
  attempts + 1, claim_token = gen_random_uuid() WHERE id = (SELECT id … WHERE runnable
  ORDER BY run_after, id FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING …`. Runnable: pending
  and past `run_after`, or running with an expired lease (its worker died), and under
  `max_attempts`. The work happens outside any transaction: no row lock is ever held
  across a model call.
- **Fencing.** Complete and fail are `… WHERE id = $1 AND claim_token = $2`. A worker
  whose lease expired and whose item was claimed again (new token), or re-queued by
  reconcile (no token), cannot complete or fail it; its late acknowledgement changes
  nothing and is logged.
- **Failures.** A handler error is caught per item and recorded in `last_error`
  (truncated to 500 characters; only the exception's class, plus the message of the
  `ItemError` type whose messages are written to be safe, never posting text or model
  output). The item is retried with backoff (30 s × 2^(attempt−1), at most 1 h) up to
  `max_attempts` (5), then `failed`. An expired lease on the last attempt also becomes
  `failed`. A failed item is the job's `needs_review` mark: never retried until its
  posting changes. Only fatal errors stop the worker: bad configuration, or the database
  unreachable for longer than a minute; Compose restarts it.
- **Polling** with the worker's poll interval when the queue is empty, no wait while
  there is work. `LISTEN/NOTIFY` may be added later to reduce latency.
- **Visibility:** counts by state through `collector queue` and `GET /api/queue`.
- **Grants:** `jobhunt_app` gets SELECT, INSERT, UPDATE and DELETE on `work_items` (the
  default privileges of ADR-0004's roles), nothing more; the owner role owns it.

## Alternatives considered

- **Redis + BullMQ-style queue:** familiar, but adds a service, a second source of truth,
  and a dual-write problem between the posting row and the queue message.
- **River (Go library on Postgres):** excellent, but Go-only; the consumers are Python.
- **NATS / RabbitMQ:** far beyond what this volume needs.
- **Holding `FOR UPDATE` for the whole item (the first version of this ADR):** simple,
  but a model call would hold a transaction and a connection open for tens of seconds,
  and a stuck call would block vacuum. Leases with a token give the same exclusivity.
- **Enqueue in the posting's transaction (the first version):** the collector does not
  know whether the default preset shows a posting until its group is evaluated; enqueuing
  every posting would pay for ~5,900 jobs instead of ~500.
- **A row per attempt, completed rows archived:** more history, but the table grows with
  every change; the done row per posting is enough, and `model_calls` (M3-03) keeps the
  cost history.

## Consequences

- **Good:** one stateful service; queue state is queryable with SQL; one backup covers
  everything; no long transactions; a dead worker's item comes back after its lease.
- **Bad:** we write lease, retry and dead-letter logic ourselves; a handler may run
  twice for one item if a lease expires mid-call (the second claim wins and the first
  ack is rejected), so handlers must be safe to repeat; polling adds small latency.
- **Revisit when:** sustained throughput needs above ~100 jobs/second, or a third
  consumer type with different delivery semantics appears.
