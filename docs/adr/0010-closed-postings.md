# 0010. Close postings missing from a successful full-board fetch

- **Status:** Accepted
- **Date:** 2026-11-02
- **Story:** docs/stories/M2-04-closed-posting-detection.md

## Context

Every collector run fetches each company's whole board (Lever, Greenhouse and Ashby list
all published postings in one response), so a stored posting that is missing from a
successful fetch has been taken down. Nothing records that today: removed roles stay in the
leads forever. Closing wrongly is the expensive mistake (a live role hidden from the owner),
and the likely cause is an outage or API change that returns an empty or broken board.

## Decision

- **`postings.closed_at timestamptz NULL`**: set when a posting is missing from a successful
  fetch of its company's board, cleared when it reappears (counted as reopened and logged by
  posting ID). Rows are never deleted; postings stay the record of what was seen.
- Closing runs **in the company's transaction, after its upserts, only when the fetch
  succeeded**. A failed, skipped or cancelled fetch returns before the transaction, so it can't
  close anything.
- **Empty-board guard:** a successful fetch with 0 postings for a company that has more than
  `collector.EmptyBoardGuard` (10) open postings closes nothing and fails that company with
  "empty board, refusing to close N postings". It shows up in `collector_run_errors` and makes
  the run `partial`.
- **Ratio guard:** a fetch that would close **more than half** of a company's open postings
  **and at least** `collector.CloseRatioGuardMin` (10) closes nothing and fails the company with
  "refusing to close N of M postings": a partial outage must not close most of a board. It
  also refuses an empty board for exactly 10 open postings (10 of 10); the empty-board guard
  covers more than 10, and fewer than 10 may close.
- **Skipped entries still count as seen:** adapters report the IDs of entries they skip as
  malformed or unstorable (a NUL posting, say) whenever the ID is readable, and those
  postings are never closed.
- `collector_runs` gains **`postings_closed`** and **`postings_reopened`** (ADR-0006's "add
  columns").
- **`job_groups`** prefers the open member seen first as canonical, and gains `closed`: true
  only when every member is closed (ADR-0009 anticipated this).
- M2-03 turns `closed_at` into the `closed` facet, hidden by the default preset.

## Alternatives considered

- **Recheck each posting's URL:** one request per posting instead of per board, against the
  boards' HTML pages; the full-board fetch already answers the question.
- **Delete missing postings:** loses history and breaks tracked applications (M6).
- **A ratio guard alone, without the minimum:** small boards turn over by more than half
  legitimately (2 of 3 roles filled), so the guard also requires at least 10 closures.

## Consequences

- **Good:** leads drop closed roles automatically; reopened roles come back; an empty or broken
  response can't close a board.
- **Bad (known limits):** an entry skipped with **no readable ID** looks missing and is closed
  until it is readable again; a partial outage that drops at most half of a board, or fewer than
  10 postings, still closes them.
- **"Closed" means "no longer published on the board", not always "URL dead".** On
  2026-11-02 a closed Spotify posting was gone from Lever's list and its single-posting API
  (`404 Document not found`) but its `jobs.lever.co` page still rendered: Lever keeps unlisted
  or internal postings reachable by direct link. Closed Greenhouse postings redirect to the
  board with `?error=true`.
- **Revisit when:** a run closes an unusual share of a board (watch `postings_closed`), or a
  board starts paginating.
