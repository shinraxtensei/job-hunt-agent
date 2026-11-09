# 0029. Board-wide sources: postings of many companies from one board

- **Status:** Accepted
- **Date:** 2026-11-09
- **Story:** docs/stories/M7-03-remote-boards.md

## Context

Every source so far is a company feed: one watch-list row (ADR-0005) is one board, and
every posting it returns belongs to that company. Remote job boards (Remotive, RemoteOK)
return postings of many companies in one fetch. Their terms ask for a link back and the
board named as the source, and Remotive asks for at most ~4 fetches a day. Three things
in the collector assume one company per fetch: the company of each stored posting,
closing what a fetch no longer lists (ADR-0010, scoped to the company), and grouping
(ADR-0009, by company and title key).

## Decision

**The board is a watch-list row.** `data/companies.yaml` gets one entry per board
(`board_type: remotive`, a fixed slug), collected like any company. Its adapter returns
each posting with the posting's company name (`job.Raw.Company`).

**Company per posting.** A board posting is stored under the company whose name matches
case-insensitively, preferring a watch-list row (a company with a board), so the same
job on the company's own feed groups with it (ADR-0009). With no match, a board-less
company is created as for manual adds (ADR-0015).

**Origin `board`.** `postings.origin` gains `'board'`. Board postings are shown like
manual ones even though their company isn't active (`ListLeads`, `ListGroupsForPresets`),
and the preset still filters them. In a group, a company's own posting is canonical over
a board's (`job_groups` orders board postings after the others).

**Closing by source.** For a board-wide source, what the fetch no longer lists is closed
across the whole source (`source = 'remotive'`), not within one company; the empty-board
and close-ratio guards (ADR-0010) count the source's open postings.

**Minimum fetch interval.** An adapter may declare `MinInterval()`. The collector stores
`companies.fetched_at` after a successful fetch and skips a company fetched more recently
than its source's interval (not a failure). Remotive: 6 hours, so at most 4 fetches a day
however often `make collect` runs.

**Attribution.** The leads API returns the canonical posting's `source`; the tracker says
"via Remotive" for a board posting, and its link is the board's posting URL.

## Consequences

- Adding another board is an adapter plus a watch-list entry, after its terms are read.
- A company named differently on a board ("Acme Inc" vs "Acme") gets its own board-less
  row; the existing `merge-company` command joins them.
- A board posting of a company excluded on the watch list is stored under that company and
  hidden with it.
