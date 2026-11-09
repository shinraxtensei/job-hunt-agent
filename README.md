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

**v1 complete (2026-11-08):** collection, scoring, CV and cover-letter drafts from the
evidence bank (with a claim checker), one-page PDFs, and the full tracker all run
locally. Deploying to a server (milestone D), the morning digest and more sources (M7),
and feedback (M8) come next. See [`docs/roadmap.md`](docs/roadmap.md).

## How to use it daily

You need Docker, a filled-in `.env` (copy `.env.example`), and your personal files in
`data/profile/`: `skill_map.yaml`, `search_profile.yaml` and `evidence.yaml`, every line
checked by you. The `*.example.yaml` files show the format.

1. **Start the stack:** `make up`. It prints the tracker's address, e.g.
   `http://127.0.0.1:8088` (`JOBHUNT_HTTP_PORT` in `.env`).
2. **Collect today's postings:** `make collect`. New leads are extracted and scored in
   the background (a few minutes for a busy day). To add a job you found elsewhere:
   `go run ./cmd/collector add <url> --title … --company …`.
3. **Read the leads:** open the tracker. Leads are best fit first, with the reason, the
   gaps and the score breakdown in each row's details. "New" shows the last 24 hours.
   Skip what isn't for you (Skip, then a reason if you like).
4. **Ask for a draft:** in a lead's details, click **CV draft**, then **Make a draft**.
   In about 15 seconds you get a one-page CV and a cover letter, built only from your
   evidence. Every sentence shows the evidence lines it came from.
   - A **blocked** draft broke an honesty rule; the sentence and the rule are shown.
     Make a new draft, or fix `evidence.yaml` if a line is missing.
   - **Approve** a ready draft. Nothing is sent.
5. **Get the PDFs:** a minute after approving, the draft lists **CV PDF** and
   **Cover letter PDF** with Download links. Check them, then send them yourself.
6. **Mark it sent:** click **Mark as sent** on each PDF you sent. Then set the lead's
   **Status** to **Applied**. That keeps a copy of the posting as you applied to it.
7. **Track it:**
   - set the status as things happen (replied, interviewing, offer, rejected);
   - add notes and the people you talk to in the lead's details;
   - the **Board** (sidebar) shows everything by status;
   - **Follow-ups due** appears at the top 7 days after applying, with a message to
     copy. Mark it **Done**, or **Snooze** it.
8. **Stop and back up:** `make down` stops the stack; `make backup` saves the database
   and your personal files to `~/job-hunter-backups` (`make restore FILE=…` brings
   them back).

## Supported sources

- **Lever**: public Postings API (`api.lever.co/v0/postings/<site>`): every Lever board on
  the watch list, ≥1s between requests, up to 3 attempts with backoff (M1-03).
- **Greenhouse**: public Job Board API (`boards-api.greenhouse.io/v1/boards/<token>/jobs`),
  descriptions included, same politeness (M1-04).
- **Ashby**: public job posting API (`api.ashbyhq.com/posting-api/job-board/<board>`),
  listed jobs only, descriptions and compensation included, same politeness (M1-05).
- **Personio**: public XML career feed (`<company>.jobs.personio.de/xml`), e.g. Inyad (M7-02).
- **Remotive**: public API (`remotive.com/api/remote-jobs`), software and DevOps jobs of many
  companies, fetched at most every 6 hours (≤ 4 a day, as its terms ask), each job linked
  to Remotive and shown "via Remotive" (M7-03, ADR-0029).
