# Job-Hunt Agent

Finds new job postings daily, scores them against my profile, drafts tailored CVs and
cover letters from a verified evidence bank, and tracks every application to its outcome.
I review and send everything.

- **Go**: collector, source adapters, scheduler, dedup, API
- **Python**: extraction, scoring, tailoring, PDF export, digest
- **PostgreSQL**: all records and the work queue
- **TypeScript**: tracker UI

Start here: [`CLAUDE.md`](CLAUDE.md) · [`docs/prd.md`](docs/prd.md) · [`docs/adr/`](docs/adr/)

## Status

Week 0: walking skeleton.

## Supported sources

- **Lever**: public Postings API (`api.lever.co/v0/postings/<site>`): every Lever board on
  the watch list, ≥1s between requests, up to 3 attempts with backoff (M1-03).
