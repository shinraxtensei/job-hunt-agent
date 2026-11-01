# 0008. A `jobs` table, 1:1 with `postings`, holds the normalised job record

- **Status:** Accepted
- **Date:** 2026-11-01
- **Story:** docs/stories/M2-01-normalise-job-record.md

## Context

M1 stores each posting's raw board JSON in `postings.raw_payload`. M2's dedup, filters
and tracker, and later the Python agents, need the same fields in one shape whichever
board a posting came from. Normalising is deterministic parsing of untrusted JSON; it will
be refined (new boards, better rules), so records must be cheap to rebuild from the raw
payloads, and it must be possible to tell which records an older normaliser made.

Data from the 5,870 stored postings (2026-11-01) shaped the decisions below:
- Lever `workplaceType` is always `hybrid`/`onsite`/`remote`; `commitment` is free text
  (`CDI`, `Permanent`, `Stage de fin d'études`, …); every posting has an ISO `country`.
- Ashby `workplaceType` is `Remote`/`Hybrid`/`OnSite` or empty (345 of 1,489); `isRemote` is
  `true` for Hybrid jobs too, so it is not used. `addressCountry` is a country *name*.
- Greenhouse has no workplace field and no country codes; 612 of 4,067 location names
  contain "Remote"/"Anywhere" and 453 contain "Hybrid".

## Decision

**One `jobs` row per posting**, keyed by `posting_id` (primary key and foreign key to
`postings.id`, `ON DELETE CASCADE`). The table is derived data: it can be truncated and
rebuilt from `raw_payload` at any time with `collector normalise`.

| Column | Type | Meaning |
|---|---|---|
| `posting_id` | bigint PK → postings | the posting this record was made from |
| `company_id` | bigint NOT NULL → companies | copied from the posting, so queries need no join |
| `title` | text NOT NULL | as published, truncated to 500 characters |
| `url` | text NOT NULL | public posting page |
| `locations` | text[] NOT NULL DEFAULT '{}' | as published, primary first, each ≤ 500 chars, no duplicates |
| `country_codes` | text[] NOT NULL DEFAULT '{}' | ISO 3166-1 alpha-2 **only where the board gives codes** (Lever today); names stay in `locations` and M2-03 classifies them |
| `workplace` | text NOT NULL, CHECK in (`remote`,`hybrid`,`onsite`,`unknown`) | see the per-board rules below |
| `team` | text NOT NULL DEFAULT '' | as published |
| `department` | text NOT NULL DEFAULT '' | as published |
| `employment_type` | text NOT NULL DEFAULT '' | **as published**, not mapped to an enum (free text on Lever; M2-03 classifies) |
| `published_at` | timestamptz NULL | when the board says it was published; NULL if it doesn't say |
| `normaliser_version` | integer NOT NULL | `job.NormaliserVersion` that produced the row |
| `posting_hash` | bytea NOT NULL | the posting's `content_hash` the row was made from |
| `normalised_at` | timestamptz NOT NULL DEFAULT now() | when the row was written |

Empty text is `''`, never NULL, so Go and Python never have to tell the two apart; only
`published_at` is nullable.

**Workplace rules** (never guess `onsite`):
- Lever: `workplaceType` → `remote` / `hybrid` / `onsite`; anything else `unknown`.
- Ashby: `workplaceType` `Remote`/`Hybrid`/`OnSite` → `remote`/`hybrid`/`onsite`; empty →
  `unknown`. `isRemote` is ignored (true for hybrid jobs).
- Greenhouse: from the location text only. Contains "remote" or "anywhere" → `remote`;
  contains "hybrid" → `hybrid`; **both → `unknown`** (e.g. "Remote or Hybrid"); neither →
  `unknown`. This extends the story's "Remote/Anywhere" rule with "Hybrid", which is
  equally explicit and appears on 453 postings.

**Other per-board choices:** Greenhouse `department` = the first `departments[].name`
(team stays empty: Greenhouse has no team); locations = `location.name` then each
`offices[].location`. Published: Lever `createdAt` (epoch ms), Greenhouse
`first_published`, Ashby `publishedAt`.

**What stays only in `raw_payload`:** descriptions (HTML and plain), lists, compensation,
apply URLs, office and department IDs, metadata, and everything else not in the table.
M3 extraction reads descriptions from there.

**Versioning:** `job.NormaliserVersion` (an integer constant, starting at 1) is bumped
whenever the normaliser's output for the same payload changes. `collector normalise`
rewrites every row that is missing, made by an older version, or made from an older
payload (`posting_hash` ≠ the posting's `content_hash`). The collector writes the row in the
posting's transaction, only for new or changed postings (M1-07).

**Normalising never fails a company:** `job.Normalise(source, payload)` is pure (in
`internal/job`, no I/O); a missing or wrongly typed field becomes empty / `unknown`. It
errors only for a payload that isn't a JSON object or an unknown source; then the posting
is still stored and the record is skipped and logged (by posting ID only).

**Failures are counted, not just logged:** `collector_runs.jobs_failed` records how many
postings in a run could not be normalised (ADR-0006's "add columns"), and
`collector normalise` reports a failed count next to the rows it wrote.

**Later columns:** M2-02 (the dedup group key) and M2-03 (the classified facets) extend
this table under their own ADRs.

## Alternatives considered

- **Columns on `postings`:** mixes raw and derived data; rebuilding means updating the raw
  table, and the normaliser version would sit next to data it doesn't own.
- **A SQL view over `raw_payload`:** no Go code to test or fuzz, but three boards' JSON
  rules in SQL, re-run on every read, and no place for the version.
- **Mapping `employment_type` and countries to enums now:** needs tables of every board's
  spellings and country names; that is classification, and M2-03 owns it.
- **Using Ashby `isRemote`:** wrong for 340 hybrid jobs.

## Consequences

- **Good:** one shape for dedup, filters, the tracker and Python; records are disposable and
  versioned; the raw payload stays the source of truth.
- **Bad:** one more shared table (a migration and this ADR for any column change); the
  collector does a little more work per new or changed posting; free-text fields
  (`employment_type`, Greenhouse locations) still need M2-03 to be filterable.
- **Revisit when:** a board offers structured locations or country codes worth parsing, or
  Python needs fields from the description (then M3, not this table).
