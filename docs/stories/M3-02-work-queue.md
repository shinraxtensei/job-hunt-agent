# M3-02: Work queue claim and retry

- **Status:** Ready
- **Milestone:** M3 — Extraction and scoring
- **Linear:** [HAM-41](https://linear.app/hamid-corp/issue/HAM-41/m3-02-work-queue-claim-and-retry)
- **Estimate:** 1 day

## Story

As the job seeker, I want new and changed leads my default filters show to be queued for
extraction exactly once and processed even across restarts, so that every lead I see gets
facts and a score without paying for jobs I never look at.

## Context

ADR-0001 (Postgres as the work queue, `SELECT … FOR UPDATE SKIP LOCKED`) is still
Proposed: this story builds it and **Accepts or revises** it. Model calls are only for
leads the `default` preset shows (~500 of ~5,900 jobs), never for every job. The
`work_items` schema is the Go ↔ Python contract (golden rule: changed only by migration +
ADR).

## Acceptance criteria

1. **Given** the reconcile step: "enqueue an `extract` item for every group the
   **default preset shows** whose canonical posting has no extraction for its current
   `content_hash`"
   **When** it runs, which is after `collector run` (and `make collect`), after
   `collector add`, at worker start, and on demand with `collector enqueue` (for a
   preset change)
   **Then** exactly those groups get an item (new or changed postings), in one
   transaction per batch, never more than one live item per posting and kind (unique
   constraint); running it twice, or from two places at once, enqueues nothing more
   (test); jobs the preset hides are never enqueued (test with a hidden job).
2. **Given** several workers
   **When** they claim at the same time
   **Then** each item is claimed by exactly one (`FOR UPDATE SKIP LOCKED`), with a
   visibility timeout; an item whose worker died is claimable again after it (test: a
   claim that is never acknowledged).
3. **Given** a failing item
   **When** it fails
   **Then** it is retried with backoff up to a maximum number of attempts, then marked
   `failed` with the last error (truncated, no posting text) and the job is marked
   `needs_review`; completed items are deleted or archived so the table stays small.
4. **Given** the worker restarts mid-item
   **When** it comes back
   **Then** nothing is lost or done twice to completion (test kills the worker between
   claim and acknowledge).
5. **Given** the queue
   **When** I look at it
   **Then** `collector`/API expose counts by state (pending, running, failed) and the
   M2 tracker is unaffected.

## Out of scope

- What the worker does with an item (M3-04, M3-06). LISTEN/NOTIFY (polling with backoff
  is enough).

## Security notes

Queue rows carry ids, never posting text. Errors stored are truncated and never include
model output or posting text.

## Notes for the implementer

Tests on both sides against real Postgres (testcontainers in Go, pytest with the same
pinned image in Python). ADR-0001 → Accepted (or a superseding ADR) in this PR.
