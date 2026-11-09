# M8-02: Collector benchmarks

- **Status:** Done
- **Milestone:** M8 — Feedback and write-up
- **Linear:** [HAM-69](https://linear.app/hamid-corp/issue/HAM-69/m8-02-collector-benchmarks)
- **Estimate:** ½ day

## Story

As the job seeker showing this project, I want measured numbers for the collector, so
that the README can say how fast it is and where the time goes, with evidence.

## Context

The PRD asks for a collector that is polite first and fast second. A real run on the Mac
(52 companies, ~6,000 postings) takes ~23 s. Ashby hosts 24 of those companies and each
host gets one request a second (`HostInterval`), so ~23 s of the run is waiting, by
design. The benchmarks separate the code's cost from the waiting.

## Acceptance criteria

1. **Given** each adapter's recorded golden file
   **When** `make bench` runs
   **Then** a Go benchmark reports parse time, bytes and allocations per response for
   Lever, Greenhouse, Ashby, Personio, Remotive and Remote OK.
2. **Given** the normaliser and classifier
   **When** `make bench` runs
   **Then** benchmarks report the cost per posting of `job.Normalise`, the classifier and
   `TitleKey`.
3. **Given** a real Postgres (testcontainers) and a fake board of 1,000 postings
   **When** the integration benchmark runs
   **Then** it reports storing a board of new postings, and re-storing it unchanged (the
   daily case), per posting.
4. **Given** the collector's run records
   **When** `docs/benchmarks.md` is written
   **Then** it shows the machine, the benchmark numbers, the last real runs, and the
   arithmetic of the rate-limit floor, with the command to reproduce each.

## Out of scope

- Making the collector faster: the floor is politeness, not code.
- Benchmarks in CI (numbers vary by machine; CI only compiles them).

## Security notes

Benchmarks read committed golden files and a throwaway database. No network, no personal
data. The real-run numbers are counts and durations only.

## Results

`docs/benchmarks.md` (2026-11-09, M2 Pro): the real 23 s run is the Ashby rate-limit floor
(24 companies, 1 request/s). Parsers run at 190–230 MB/s (Personio XML 61 MB/s);
normalise + classify ≈ 35 µs a posting; storing costs 420 µs a new posting and 188 µs an
unchanged one, ~1.1 s of database work for a daily run. `make bench` reproduces it;
`storetest.Migrate` now takes `testing.TB` so benchmarks can use it.
