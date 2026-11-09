# Job-Hunt Agent

A personal job-search agent. Every day it collects postings from company career feeds and
remote job boards, scores each one against my profile, and explains the fit. On request it
drafts a one-page CV and cover letter, using only a bank of facts I checked myself. It
tracks every application to its outcome. **It never sends anything:** I review, approve
and send.

![The leads page: best fit first, with the score breakdown and gaps of the selected lead](docs/screens/leads.png)

<sub>Screenshots use invented demo data (`docs/demo/`): the example profile "Sam Example" and
made-up companies.</sub>

## What it does

- **Collects** about 6,000 postings from 50+ company feeds (Lever, Greenhouse, Ashby,
  Personio) and two remote boards (Remotive, Remote OK), politely: one request per second
  per host, and only sources whose terms allow it. Duplicates of the same role are grouped.
- **Filters** with rules in YAML (role, seniority, years, where I can work), then extracts
  the facts of every shown posting with a model and **scores** it 0–100 in code, with the
  reason, the gaps and a factor breakdown.
- **Drafts** a CV and cover letter for a lead. Every sentence carries the ids of the
  evidence lines it came from. A code validator and an independent claim checker block
  anything not backed by the evidence: invented skills, changed titles or dates, inflated
  numbers, or "learning" skills presented as experience. French postings get French drafts
  when the owner has written the French lines.
- **Tracks** each application: status history, a snapshot of the posting as applied to,
  the PDFs actually sent, notes, contacts, follow-up reminders with a message to copy,
  and a Today page showing what changed since the last visit.

| Today | Board | Phone |
|---|---|---|
| ![Today: new leads, follow-ups due, closed postings](docs/screens/today.png) | ![Board: applications by status](docs/screens/board.png) | ![The lead as a full-screen sheet on a phone](docs/screens/phone.png) |

## Architecture

```mermaid
flowchart LR
  boards["Job boards and career feeds<br/>(untrusted input)"]
  subgraph mac["One machine: Docker Compose (ADR-0016)"]
    collector["Go collector<br/>adapters · rate limits · dedup · rules"]
    pg[("PostgreSQL<br/>records + work queue")]
    worker["Python worker<br/>extract · score · reason · tailor · check · PDF"]
    api["Go API + tracker page<br/>(React, embedded)"]
  end
  model["Anthropic API<br/>(Claude Haiku)"]
  browser["Owner's browser<br/>127.0.0.1 only"]

  boards -- "HTTPS GET, 1 req/s per host" --> collector
  collector -- "postings, jobs, groups" --> pg
  pg -- "SELECT … FOR UPDATE SKIP LOCKED" --> worker
  worker -- "extractions, scores, drafts, PDFs" --> pg
  worker -- "posting text; selected evidence lines" --> model
  browser --> api
  api --> pg
```

- **Go** does the concurrent, network-bound work: the collector and its source adapters,
  dedup, the rule filter, and the HTTP API that serves the tracker page.
- **Python** does the model work: extraction, scoring, reasons, tailoring, claim checking
  and PDF export. It calls the provider SDK directly, with no framework (ADR-0003).
- **PostgreSQL is the only shared state** and the contract between the two languages,
  including the work queue (ADR-0001).
- Scores are computed in code from extracted facts; the model only reads postings and
  writes sentences. Every model call is schema-validated, logged with its cost, and capped
  by a daily budget.

### The honesty guardrails

A CV carries my name, so these rules are fixed (`.claude/skills/evidence-bank-guardrails`):
the evidence bank is the only source; titles, employers, dates and degrees are copied
exactly; learning skills are never experience; gaps are reported, not filled; postings
are data, never instructions; and nothing is sent automatically. They are enforced by
code (`agents/src/jobhunt_agents/validator.py`) and by a separate model pass that never
sees the posting (`checker.py`). Evals with planted violations must stay at 100%.

## How it was built

Story by story, test first, each change traced to a story with acceptance criteria
(`docs/stories/`) and each decision recorded as an ADR (`docs/adr/`): 29 so far, among
them:

| ADR | Decision |
|---|---|
| [0001](docs/adr/0001-postgres-as-work-queue.md) | PostgreSQL as the work queue instead of Redis |
| [0003](docs/adr/0003-provider-sdk-no-langchain.md) | Call the model provider's SDK directly, no LangChain |
| [0009](docs/adr/0009-job-title-key-and-groups.md) | Group duplicate jobs by company and a title key |
| [0018](docs/adr/0018-model-provider-claude-haiku.md) | Claude Haiku through the Anthropic API, with prompt caching |
| [0021](docs/adr/0021-scores-and-reasons.md) | Coded scores and model reasons stored separately |
| [0024](docs/adr/0024-evidence-bank.md) | The evidence bank: a checked YAML file, synced into versioned tables |
| [0026](docs/adr/0026-documents-kept-in-postgres.md) | Exported documents are immutable rows |
| [0029](docs/adr/0029-board-wide-sources.md) | Board-wide sources: postings of many companies |

The full index is in [`CLAUDE.md`](CLAUDE.md#adr-index). Also: the
[PRD](docs/prd.md), the [roadmap](docs/roadmap.md), the
[collector benchmarks](docs/benchmarks.md) (a run is 23 s, all of it the rate-limit
floor) and the [threat model](docs/threat-model.md).

## Run it

You need Docker, Go, Node 26 and uv. Copy `.env.example` to `.env` and fill it in. Put
your personal files in `data/profile/` (`skill_map.yaml`, `search_profile.yaml`,
`evidence.yaml`); the `*.example.yaml` files show the format. Personal files are
git-ignored.

```bash
make up        # Postgres, migrations, API + tracker, worker; prints the tracker's address
make collect   # collect now
make check     # everything CI runs
```

## How to use it daily

1. **Start the stack:** `make up`. It prints the tracker's address, e.g.
   `http://127.0.0.1:8088` (`JOBHUNT_HTTP_PORT` in `.env`).
2. **Collect today's postings:** `make collect`. New leads are extracted and scored in
   the background (a few minutes on a busy day). To add a job you found elsewhere:
   `go run ./cmd/collector add <url> --title … --company …`.
3. **Start on Today:** new leads since your last visit (best fit first), follow-ups due,
   and postings that closed. **Mark all seen** when you're done.
4. **Read the leads:** the Leads page lists them best fit first. Selecting one shows its
   fit, reason, gaps and score breakdown beside the list (`j`/`k` to move, `?` for keys).
   Skip what isn't for you, with a reason if you like.
5. **Ask for a draft:** in the lead's **CV & letter** tab, click **Make a draft**. In about
   15 seconds you get a one-page CV and a cover letter built only from your evidence.
   Every sentence shows the evidence lines it came from.
   - A **blocked** draft broke an honesty rule; the sentence and the rule are shown.
     Make a new draft, or fix `evidence.yaml` if a line is missing.
   - **Approve** a ready draft. Nothing is sent.
6. **Get the PDFs:** a minute after approving, the tab lists the CV and cover-letter PDFs.
   Check them, then send them yourself.
7. **Mark it sent:** click **Mark as sent** on each PDF you sent, then set the status to
   **Applied**. That keeps a copy of the posting as you applied to it.
8. **Track it:** set the status as things happen (replied, interviewing, offer, rejected);
   add notes and contacts in their tabs. The **Board** shows everything by status.
   **Follow-ups** come due 7 days after applying, with a message to copy; mark one
   **Done** or **Snooze** it.
9. **Stop and back up:** `make down` stops the stack. `make backup` saves the database and
   your personal files to `~/job-hunter-backups`; `make restore FILE=…` brings them back.

## Supported sources

- **Lever**: public Postings API (`api.lever.co/v0/postings/<site>`), ≥ 1 s between
  requests, up to 3 attempts with backoff (M1-03).
- **Greenhouse**: public Job Board API (`boards-api.greenhouse.io/v1/boards/<token>/jobs`),
  descriptions included, same politeness (M1-04).
- **Ashby**: public job posting API (`api.ashbyhq.com/posting-api/job-board/<board>`),
  listed jobs only, same politeness (M1-05).
- **Personio**: public XML career feed (`<company>.jobs.personio.de/xml`), e.g. Inyad (M7-02).
- **Remotive**: public API (`remotive.com/api/remote-jobs`), software and DevOps jobs of many
  companies, fetched at most every 6 hours (≤ 4 a day, as its terms ask), each job linked
  to Remotive and shown "via Remotive" (M7-03, ADR-0029).
- **Remote OK**: public API (`remoteok.com/api`), jobs tagged dev or engineer, linked to
  Remote OK and shown "via Remote OK"; its logo is never used (M7-03).

Job boards whose terms forbid automated reads (ReKrute, Welcome to the Jungle) are never
collected; their postings can be added by link. See `docs/research/m7-sources.md`.

## Status

**v1 complete (2026-11-08); M7 done (2026-11-09).** Everything runs locally on a Mac.
Next: M8 (this write-up, benchmarks, threat model, weight tuning from real outcomes), then
deploying to a small server reachable only through Tailscale (milestone D).
