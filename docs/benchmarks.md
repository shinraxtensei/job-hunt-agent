# Collector benchmarks (M8-02)

Measured 2026-11-09 on an Apple M2 Pro (12 cores, 16 GB), Go 1.27, Postgres from the image
pinned in `deploy/compose.yaml` running in Docker Desktop. Reproduce with `make bench`.

## In short

A real collection of 52 companies and about 6,000 postings takes **about 23 seconds**,
and **about 22 of those are waiting on purpose**. Each board host gets at most one request
a second, and Ashby hosts 24 of the companies. The code's own work is about 1–2 seconds,
spread over 8 workers.

## Real runs

From `collector_runs` (ADR-0006), the last eight runs on the Mac:

| Run | Outcome | Companies | Postings seen, new or changed | Seconds |
|---|---|---|---|---|
| 16 | ok | 52 | 6,051 | 23 |
| 15 | ok | 51 | 5,986 | 23 |
| 14 | ok | 52 | 5,991 | 23 |
| 13 | ok | 51 | 5,980 | 23 |
| 12 | ok | 50 | 5,955 | 23 |
| 11 | ok | 50 | 5,863 | 24 |
| 10 | partial | 50 | 5,710 | 22 |
| 9 | ok | 50 | 5,858 | 22 |

```sql
SELECT id, outcome, companies_attempted, postings_new + postings_seen + postings_changed,
       extract(epoch FROM ended_at - started_at)::int
FROM collector_runs WHERE ended_at IS NOT NULL ORDER BY id DESC LIMIT 8;
```

## The rate-limit floor

`collector run` spaces requests to one host at least `HostInterval` (1 s) apart. Each
company is one request, so a run can't finish sooner than its busiest host:

| Host | Companies | Floor |
|---|---|---|
| Ashby | 24 | 23 s |
| Greenhouse | 22 | 21 s |
| Lever | 4 | 3 s |
| Personio, Remotive, Remote OK | 1 each | 0 s |

Hosts run in parallel, so the floor is the largest: **23 s**, which is what the runs
measure. Remotive and Remote OK are also fetched at most every 6 hours (ADR-0029).

## Code cost per step

`go test -bench` on the recorded golden files (`make bench`, first part):

| Step | Time | Allocations |
|---|---|---|
| Parse a Lever response | 230 MB/s, ~8 µs a posting | 8 per response |
| Parse a Greenhouse response | 233 MB/s, ~7 µs a posting | 15 per response |
| Parse an Ashby response | 221 MB/s, ~6 µs a posting | 13 per response |
| Parse a Personio feed (XML) | 61 MB/s, ~31 µs a posting | 796 per response |
| Parse a Remotive response | 232 MB/s, ~9 µs a posting | 24 per response |
| Parse a Remote OK response | 191 MB/s, ~8 µs a posting | 55 per response |
| `job.Normalise` (Greenhouse, with description) | 15 µs a posting | 107 |
| `job.Classify` (role, seniority, years, places) | 19 µs a posting | 35 |
| `job.TitleKey` | 3.5 µs a title | 34 |

Parsing and classifying a 1,000-posting board costs about 40 ms. XML is the slowest
parser, and Personio feeds are small.

## Storing postings

`make bench`, second part: one board of 1,000 postings through `collector.Once` against a
real Postgres (testcontainers), one transaction per board, as in production:

| Case | Per posting | A 1,000-posting board |
|---|---|---|
| First time seen (insert, normalise, classify, job record) | 420 µs | 0.42 s |
| Seen again, unchanged (the daily case: hash compare only) | 188 µs | 0.19 s |
| Every posting changed (update, re-normalise, rewrite the job) | 449 µs | 0.45 s |

Each posting is one round trip to Postgres in the board's transaction. On a normal day
almost everything is unchanged: 6,000 postings × 188 µs ≈ 1.1 s of database work, spread
over 8 concurrent companies.

## What this means

- The collector is bound by politeness, not by code or the database. Making it faster
  would mean asking boards for more than one request a second, which we don't.
- Adding a company to a busy host adds a second to every run; adding one to a new host
  adds nothing.
- Benchmarks run locally, not in CI: the numbers depend on the machine. CI still compiles
  them (`go vet` and `go test` build the benchmark files).
