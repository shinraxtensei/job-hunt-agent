# 0020. Extractions stored per content and prompt version; the worker records its prompt version

- **Status:** Accepted
- **Date:** 2026-11-05
- **Story:** docs/stories/M3-04-extraction-agent.md

## Context

M3-04 stores what the extraction agent finds, and two of its acceptance criteria shape
the storage:

- **AC4:** the same posting content with the same prompt version must never be sent to
  the model twice.
- **AC7:** `collector enqueue --reextract` (Go) finds the shown postings extracted with
  an older prompt. Go doesn't read the prompt files, which live in the Python package.

This is a new shared table, and it changes a data contract: Python writes it, Go reads it
(and so will the API, in M3-08).

## Decision

- **`extractions`** holds one row per (posting, content_hash, prompt_version), with a
  unique constraint on those three. It also stores the model and `data`, the validated
  `Extraction` as `jsonb`.
  - The handler checks for that key before calling the model, and inserts with
    `ON CONFLICT DO NOTHING`, so a re-queued item costs nothing (AC4).
  - A new prompt version adds rows. The old ones stay, so versions can be compared.
  - The latest version for a posting's current content is its extraction.
- **`agent_prompts (agent, version)`** is written by the worker at start-up, with the
  prompt version each agent runs (`PROMPT_VERSION` in `extract.py`).
  - `--reextract` compares against it: it re-queues the shown postings whose current
    content has extractions only with older versions.
  - Without a row it refuses: "start the worker once".
- **Estimated cost** for `--reextract` is the count × the mean `cost_usd` of successful
  `extract` calls in `model_calls`. This is measured per posting, not computed from the
  token price table.
- **No column-contract file**, unlike `work_items`. Go reads only `posting_id`,
  `content_hash` and `prompt_version`, and the Go integration tests insert rows the way
  the worker does. If the API starts reading `data` (M3-08), that story adds a contract
  test.

## Consequences

- **Re-extraction is explicit and cheap to reason about.** Shipping a prompt never
  re-queues anything by itself, and the dry run shows the count and cost first.
- **Starting order:** a worker must have started once before `--reextract` works. On
  the Mac and the server, `make up` does that.
- **Rows accumulate,** one per prompt version per content: a few thousand rows a year at
  this scale. Pruning old versions is left until it matters.
- **needs_review stays derived** from a failed work item (ADR-0001). No extraction row is
  written for it (AC2).
