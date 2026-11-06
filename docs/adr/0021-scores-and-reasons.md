# 0021. Store coded scores and model reasons separately; the worker queues its own reasons

- **Status:** Accepted
- **Date:** 2026-11-06
- **Story:** docs/stories/M3-06-scoring-agent.md

## Context

M3-06 adds what the tracker shows per lead (M3-08 reads it): a score computed in code, a
fit level, and a reason, gaps and CV variant written by the model. The story's criteria
pull the two apart:

- **AC4:** a change of weights, bands or profile recomputes every shown lead's score at
  once, with no model call.
- **AC5:** the reason is regenerated lazily. That happens at once for new posting
  content, otherwise only when the lead is opened; until then the old reason is shown,
  marked as older.
- **Freshness** must not go stale in storage.

The work queue (ADR-0001) is the only place with retries, needs_review and the retry
command. Until now only Go enqueued (`extract`), because only Go evaluates presets.

## Decision

- **`scores`:** one row per shown posting, recomputed in place when its inputs change.
  - It holds the weighted points without freshness, the per-factor breakdown (`factors`
    jsonb), `not_enough_info`, and the versions it came from: weights, bands, profile,
    extraction prompt.
  - It also stores the freshness and bands parameters it was scored with.
  - `reason_inputs` hashes what a reason is made from: factors, profile, reason prompt.
  - `reason_requested_at` is set when the lead is opened (M3-08).
- **`score_reasons`:** one row per posting with reason, gaps, CV variant and model, plus
  the posting content and the `reason_inputs` it was made from.
- **`lead_scores` view:** adds freshness as of `now()` and the fit level from the stored
  bands. `reason_current` says whether the reason matches the score's current inputs.
- **Scoring needs no queue item.** On every iteration the worker recomputes scores whose
  inputs changed, in one pass with no model call.
  - "Shown" means the posting has an `extract` work item, which Go queues only for leads
    the default preset shows.
- **A new `reason` work-item kind, which the worker enqueues itself.** It is queued:
  - when a score's posting content has no reason yet;
  - when `reason_requested_at` is newer than the reason and the reason is not current.

  Claiming, retries and needs_review stay as in ADR-0001.
- **The reason agent never sees posting text.** It gets the coded breakdown only (factor
  values, skills and their levels). Its gaps must be skills the breakdown lists below
  proven, or the output is invalid.
  - The CV variant is a `decide()` call over the title and the extracted skills.

## Alternatives considered

- **One table with score and reason:** a weights change would have to null or flag the
  reason in the same write, and "shown as older" would need an extra column anyway.
- **Store the total with freshness:** stale the next day, or a daily rewrite of every row.
- **Go enqueues `reason` items:** Go would have to know the score's inputs and the reason
  prompt version, which live in Python.
- **A Python-side status column instead of a queue kind:** it would duplicate retries,
  needs_review and the retry command.

## Consequences

- **Good:**
  - A weights or profile change costs nothing and reaches every lead in one pass.
  - The reason can't be steered by posting text, since it never reads any.
  - Lazy regeneration keeps model cost proportional to what the owner opens.
- **Bad:**
  - Python now writes `work_items` rows too, so the contract has two writers.
  - The view repeats `score.freshness` and `score.band` in SQL; the integration test
    compares the two.
  - A lead hidden by a later preset change keeps its score, because the shown set is
    "has an extract item". The API filters by preset anyway.
- **Revisit when:** a second scoring agent or per-preset scores are needed, or the
  shown-set approximation shows stale leads.
