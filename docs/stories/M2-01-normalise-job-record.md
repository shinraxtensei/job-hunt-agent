# M2-01: Normalise postings into the job record

- **Status:** Done
- **Milestone:** M2 — Rules and a plain tracker
- **Linear:** [HAM-30](https://linear.app/hamid-corp/issue/HAM-30/m2-01-normalise-postings-into-the-job-record)
- **Estimate:** 1 day

## Story

As the job seeker, I want every stored posting turned into one common job record,
whichever board it came from, so that rules, deduplication and the tracker work the
same way for Lever, Greenhouse and Ashby.

## Context

M1 stores each posting's raw payload (`postings.raw_payload`) plus four columns
(source id, URL, title, company). Everything else (locations, remote policy, team,
employment type, published date) is still inside board-specific JSON. M2's rules and
tracker need those fields in one shape. PRD: "Normalise. Convert each posting to one
common record". This is deterministic parsing, not AI: extraction of skills, years and
visa support stays in M3.

What each board provides (checked against the golden files on 2026-11-01):

| Field | Lever | Greenhouse | Ashby |
|---|---|---|---|
| Locations | `categories.location`, `categories.allLocations`, `country` | `location.name`, `offices[].location` | `location`, `secondaryLocations`, `address.postalAddress.addressCountry` |
| Workplace | `workplaceType` (onsite/hybrid/remote) | none (infer from location text) | `workplaceType`, `isRemote` |
| Team / department | `categories.team`, `categories.department` | `departments[].name` | `team`, `department` |
| Employment type | `categories.commitment` | none | `employmentType` |
| Published | `createdAt` (epoch ms) | `first_published` | `publishedAt` |

## Acceptance criteria

1. **Given** a raw posting from Lever, Greenhouse or Ashby
   **When** it is normalised
   **Then** the job record holds: posting id, company id, title, locations (list of
   strings as published), country codes where the board gives them, workplace
   (`remote` / `hybrid` / `onsite` / `unknown`), team, department, employment type,
   published-at, URL, and the normaliser version that produced it.
2. **Given** the golden files of all three adapters
   **When** the table-driven tests run
   **Then** each produces exactly the expected record (one test table per board, at least
   one remote, one hybrid and one multi-location case each).
3. **Given** a field is missing, null or the wrong type in the payload
   **When** it is normalised
   **Then** that field is empty / `unknown` and the record is still produced. Normalising
   never fails a whole company and never panics (fuzz test seeded with the golden files).
4. **Given** the collector stores a new posting or one whose `content_hash` changed
   **When** the company's transaction commits
   **Then** its job record is created or replaced in the same transaction. An unchanged
   posting's record is not rewritten.
5. **Given** the 5,870 postings already stored, or a new normaliser version
   **When** I run `go run ./cmd/collector normalise`
   **Then** every posting whose record is missing or was made by an older normaliser
   version is (re)normalised, in batches, and the command reports counts.

## Out of scope

- Deduplication (M2-02), rules (M2-03), anything that needs a model (M3).
- Rendering or storing descriptions as text: the raw payload already holds them.

## Security notes

Payloads are untrusted: the normaliser reads them as data only, caps string lengths it
copies (titles and locations over 500 characters are truncated), and never logs them.
New table shared with the Python agents later → ADR (data contract), per the `adr` skill.

## Notes for the implementer

- Pure function in `internal/job` (`Normalise(source string, payload []byte) (Record, error)`),
  one small file per board. `internal/job` imports nothing from store/source.
- Proposed storage: a `jobs` table 1:1 with `postings` (posting_id primary key), so it can
  be dropped and rebuilt from raw payloads at any time. Settle it in the ADR.
- Greenhouse has no workplace field: infer `remote` only from explicit words in the
  location text ("Remote", "Anywhere"), otherwise `unknown`, never guess `onsite`.
