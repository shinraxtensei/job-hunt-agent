# 0015. Manual postings: origin on postings, board-less companies, recheck by posting

- **Status:** Accepted
- **Date:** 2026-11-02
- **Story:** docs/stories/M2-07-manual-add-by-link.md

## Context

M2-07 lets the owner add a job by its link (`collector add <url>`). Two kinds of links:
a posting on a supported board (Lever, Greenhouse, Ashby), which can be fetched through
that board's public API; and anything else (LinkedIn, a careers page), which must never
be fetched: the owner types the title and company. Today every posting comes from a
collected board, every company has a board, and closing works only by full-board fetch
(ADR-0010). A pasted URL is user input that could point anywhere: SSRF is the risk.

## Decision

**Fetching (the SSRF rule, absolute).** `job.ParseBoardLink` accepts a link as a board
posting only when, parsed with `net/url`: the scheme is `https`; there is no user info, no
port, no percent-encoded path; the lower-cased host is *exactly* `jobs.lever.co`,
`boards.greenhouse.io`, `job-boards.greenhouse.io` or `jobs.ashbyhq.com`; and the path is
that board's posting shape with a board slug of `[A-Za-z0-9._-]` and the board's id format
(UUID for Lever and Ashby, digits for Greenhouse). The adapter then fetches **its own
fixed API endpoint** built from that board and id (`api.lever.co/v0/postings/<site>/<id>`,
`boards-api.greenhouse.io/v1/boards/<board>/jobs/<id>`; Ashby has no single-posting
endpoint, so its board list is fetched and the id picked). The pasted URL itself is never
requested. An `http://` link on a board host is refused with "use the https:// link"
(owner decision), so it can't become a manual duplicate of a board posting. Every other
link, IP addresses and look-alike hosts included, is a manual link: stored, never fetched.

**Schema** (migration `manual_postings`):

- `postings.origin text NOT NULL DEFAULT 'collected' CHECK (origin IN ('collected',
  'manual'))`: how the posting entered. A manually added board posting keeps its board's
  `source` and id, so a later collected copy is the same row.
- Non-board postings use `source = 'manual'` and `source_posting_id` = the normalised link
  (lower-cased scheme and host, no fragment, no trailing slash, tracking parameters
  `utm_*`, `trk`, `ref`, `refId`, `trackingId` removed, the rest sorted). The existing
  `UNIQUE (source, source_posting_id)` is the duplicate check (AC5), for board links too.
  `raw_payload` holds what the owner typed (`title`, `company`, `location`, `url`), and the
  normaliser has a `manual` case, so these postings go through the same normalise →
  classify → group path as collected ones.
- `postings.last_rechecked_at timestamptz` (NULL until rechecked).
- `companies.board_type` and `board_slug` become nullable, both or neither
  (`CHECK ((board_type IS NULL) = (board_slug IS NULL))`): a company typed with
  `--company` has no board. Board-less companies are unique by `lower(name)` (partial
  unique index). Companies created by `add` are `active = false`, so they are never
  collected daily unless added to the watch list.
- `collector_runs.manual_rechecked integer`.

**Showing them.** The leads API (ADR-0012) shows groups of active companies **and** groups
with a manual member, so an added job appears in the tracker under the preset like any
other lead. Each lead carries `origin` (`collected` | `manual`, of its canonical posting).

**Recheck (AC4).** `collector run`, after the companies, rechecks manual *board* postings
whose company isn't collected and that weren't rechecked in the last 2 days: one
`FetchOne` each, through the run's shared per-host limiter. 404 → closed; found → stored
like a collected posting (reopened if closed). Counted in `collector_runs`. Manual
non-board postings are never fetched, so never rechecked.

## Alternatives considered

- **A separate `manual_postings` table:** a second shape for the normaliser, the groups
  view and the API; `origin` on `postings` keeps one path.
- **A placeholder board for typed companies (`board_type = 'manual'`):** avoids nullable
  columns but invents a board that doesn't exist and would need excluding everywhere.
- **Fetching the pasted URL and parsing HTML:** the SSRF risk this ADR exists to avoid;
  out of scope by the story.
- **Allow-listing by suffix or regexp on the raw string:** look-alike hosts and userinfo
  tricks (`jobs.lever.co.evil.com`, `jobs.lever.co@evil.com`) are exactly what suffix and
  string matching get wrong; exact host after `net/url` parsing is the rule.

## Consequences

- **Good:** one storage and display path for every posting; duplicates impossible by
  construction; the only outbound requests are to the adapters' fixed API hosts.
- **Bad:** queries over companies must handle board-less rows; a manual non-board job is
  never closed automatically (the owner closes it, M6).
- **Revisit when:** another board gets a link format, or adding moves to the web page (M6).
