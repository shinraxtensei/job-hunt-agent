# M2-04: Closed-posting detection

- **Status:** Ready
- **Milestone:** M2 — Rules and a plain tracker
- **Linear:** [HAM-33](https://linear.app/hamid-corp/issue/HAM-33/m2-04-closed-posting-detection)
- **Estimate:** ½ day

## Story

As the job seeker, I want postings that were taken down marked closed, and reopened if
they come back, so that I never spend time on a role that no longer exists.

## Context

The roadmap said "liveness recheck every 2 days". The collector already fetches each
company's **whole board** on every run, so a posting missing from a successful fetch is
gone: no per-posting recheck is needed for board postings. Only manually added postings
(M2-07) need a recheck. This story changes the roadmap wording accordingly.

## Acceptance criteria

1. **Given** a company whose fetch succeeded
   **When** a stored posting of that company is not in the fetched board
   **Then** it gets `closed_at` set (in the same transaction as the company's upserts),
   and the run record counts it in a new `postings_closed` column.
2. **Given** a company whose fetch failed, was skipped or was cancelled
   **When** the run ends
   **Then** none of its postings are closed. Tested with a failing fake source.
3. **Given** a closed posting
   **When** it appears again in a later successful fetch
   **Then** `closed_at` is cleared and it counts as reopened (logged with its id).
4. **Given** a successful fetch that returns 0 postings for a company that had more than
   10 open postings
   **When** it is stored
   **Then** nothing is closed and the company is recorded as an error
   ("empty board, refusing to close N postings"): an outage or API change must not close
   a whole board. The threshold is a constant with a test.
5. **Given** closed postings
   **When** jobs are classified (M2-03)
   **Then** `closed` is a facet: hidden by the default preset, visible with a filter.
6. **Given** a duplicate group (M2-02, `job_groups`) with closed and open members
   **When** the group is read
   **Then** its canonical posting is the **open** member seen first, and the group is closed
   only when **all** its members are closed. Tested with a group of one closed member seen
   first and one open member seen later (canonical = the open one), and a group of only
   closed members (closed).

## Out of scope

- Rechecking manually added postings: part of M2-07.

## Security notes

None new. The empty-board guard (AC4) is the main safety property: test it explicitly.
