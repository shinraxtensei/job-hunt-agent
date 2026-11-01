# 0001. Use PostgreSQL as the work queue instead of Redis

- **Status:** Proposed
- **Date:** 2026-10-31
- **Story:** Week 0 — walking skeleton

## Context

The PRD hands postings from the Go pipeline to the Python agents through a Redis queue.
This is a single-user system running on one small server, built and operated by one
person. Every extra service has to be deployed, monitored, backed up, and understood
when it fails at 2 a.m. PostgreSQL is already required for all records, and the owner
knows it well.

Volume is small: hundreds to low thousands of postings per day.

## Decision

Use a `work_items` table in PostgreSQL as the queue. Workers claim jobs with
`SELECT … FOR UPDATE SKIP LOCKED`, inside a transaction, with a visibility timeout and
an attempt counter. Go enqueues in the same transaction that writes the posting
(transactional enqueue, no outbox needed). Python workers poll with short backoff;
`LISTEN/NOTIFY` may be added later to reduce latency.

The `work_items` schema is the contract between Go and Python and is changed only through
a migration plus an ADR.

## Alternatives considered

- **Redis + BullMQ-style queue:** familiar, but adds a service, a second source of truth,
  and a dual-write problem between the posting row and the queue message.
- **River (Go library on Postgres):** excellent, but Go-only; the consumers are Python.
- **NATS / RabbitMQ:** far beyond what this volume needs.

## Consequences

- **Good:** one stateful service; enqueue and data write are atomic; queue state is
  queryable with SQL; one backup covers everything.
- **Bad:** polling adds small latency; we write the retry and dead-letter logic ourselves;
  heavy queue churn would bloat the table (mitigated by deleting completed rows and
  autovacuum).
- **Revisit when:** sustained throughput needs above ~100 jobs/second, or a third
  consumer type with different delivery semantics appears.
