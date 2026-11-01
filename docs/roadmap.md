# Roadmap

The whole plan, from Week 0 to Milestone 8. This file fixes **scope and order** so the
project doesn't drift. It does not fix every detail.

## How planning works (rolling wave)

- **Every milestone** has a goal, exit criteria, and a story list here. That is the scope
  contract.
- **Detailed stories** (Given/When/Then) are written only for the current and next
  milestone. Writing them earlier means writing them twice: what we learn in M1 changes
  how M3 should look.
- **At the start of each milestone**, its story titles below become full story files in
  `docs/stories/` and issues in Linear.
- **Changing scope** means editing this file in a PR, like an ADR: what changed and why.
  Nothing gets added or dropped silently.
- **Cut line:** the board view, the digest and extra sources may slip. Collection,
  scoring, tailoring and tracking may not.
- **Parallel rule:** keep applying for jobs during the build.

Timeline: part-time, roughly one milestone per week; M1 is allowed two.

---

## M0: Walking skeleton (Week 0)

**Goal:** a thin, ugly, complete path from a real job board to a local database, with
CI guarding it. Every later milestone adds muscle to this skeleton.

**Exit criteria**
- `make check` runs lint, tests with `-race`, and govulncheck, locally and in GitHub Actions.
- One real Lever posting is fetched and stored in the local Postgres.

**Stories:** M0-01 Go module and tooling · M0-02 CI pipeline · M0-03 Local Postgres and
migrations · M0-04 First Lever posting end to end

M0-05 (server and deploy) moved to **D: Deploy**, after M2.

## M1: Collector

**Goal:** postings from a 50-company watch list on Lever, Greenhouse and Ashby are
collected daily, stored raw, and never lost.

**Exit criteria**
- 50 watch-list companies in the database, loaded from a reviewed seed file.
- Lever, Greenhouse and Ashby adapters pass golden, error and fuzz tests.
- One `collector run` collects all companies concurrently with per-host rate limits;
  failures for one company don't stop the others. Run by hand, locally.

**Stories:** M1-01 Watch list seed and companies sync · M1-02 Concurrent collector run ·
M1-03 Lever adapter, complete · M1-04 Greenhouse adapter · M1-05 Ashby adapter ·
M1-07 Idempotent posting upsert · M1-10 Slow but progressing downloads

M1-06 (Workable adapter) moved to the **M7** backlog: no active watch-list company posts
on Workable (see the change log).

M1-08 (daily schedule and monitoring) and M1-09 (backups) moved to **D: Deploy**: both
only make sense on an always-on server.

## M2: Rules and a plain tracker (usable from here)

**Goal:** the owner opens one page each morning and sees today's new leads that fit his
profile, and can widen any filter with one click.

**Exit criteria**
- Every posting is normalised into the common job record.
- The same role seen twice appears once.
- Every job is classified: role family, seniority, workplace and remote scope, location,
  explicit experience minimum, closed. Nothing is deleted or marked dropped.
- Filter presets in `data/presets/` decide what is shown; `default.yaml` holds the
  owner's profile. The hidden jobs, and which filter hid each one, are always viewable.
- A posting missing from a successful full-board fetch is marked closed; manually added
  board postings are rechecked every 2 days.
- A read-only web page lists jobs newest first, with the default preset applied and
  every facet filterable, usable on a phone.

**Stories:** M2-01 Normalise to the job record · M2-02 Deduplication · M2-03 Classify
jobs and filter with presets · M2-04 Closed-posting detection · M2-05 Leads API (Go) ·
M2-06 Plain tracker table (TypeScript) · M2-07 Manual add by link

Order: M2-01 → M2-02 → M2-03 (uses M2-04's `closed` facet, can start before it) →
M2-05 → M2-06. M2-04 and M2-07 can go any time after M2-01.

## D: Deploy (after M2)

**Goal:** the usable tracker from M2 runs on an always-on server only the owner can reach,
collects every morning on its own, and its data is backed up.

**Exit criteria**
- A push to `master` deploys automatically; the server is reachable only over Tailscale.
- The collector runs daily on the server; runs are recorded and a missed run alerts the owner.
- A nightly encrypted backup exists off the server, and a restore was tested once.

**Stories:** M0-05 Server, Tailscale and deploy · M1-08 Daily schedule and run monitoring ·
M1-09 Nightly backup and tested restore (story IDs kept from their original milestones)

## S: Sharing with friends (after D, not scheduled)

**Goal:** a few friends, possibly in other fields (sales, marketing...), use the tracker
with their own filters, without seeing anything private of the owner's.

**Before anything is shared:**
- **Access:** ADR-0002 (no auth, Tailscale only) must be revisited. The cheapest option
  is sharing the server's Tailscale node with each friend (no auth code); the
  alternative is real accounts. Decide in a new ADR.
- **Privacy boundary:** collected jobs and facets are shared data; the owner's CV,
  evidence bank, scores, tailored documents and applications are never visible to
  anyone else. Needs a threat-model update.
- **Per-user presets:** a friend's filters are theirs (saved per user, not in the repo).
- **Coverage:** the watch list is chosen for the owner's targets; friends in other fields
  may want more companies. Collection stays one shared watch list.
- Scoring, tailoring and documents (M3–M5) stay owner-only.

**Stories:** written when this milestone is scheduled.

## M3: Extraction and scoring

**Goal:** every lead that passes the rules has structured facts, a score, a short
reason, gaps and a CV variant.

**Exit criteria**
- The Python worker claims work from the Postgres queue (ADR-0001) and survives restarts.
- Extraction returns a validated schema; malformed output retries once, then `needs_review`.
- Scoring follows the PRD weights, held in configuration.
- An eval set of 30–50 labelled postings exists; extraction and scoring meet agreed
  thresholds before merging prompt changes.
- Model cost per lead is logged.

**Stories:** M3-01 Python project and worker skeleton · M3-02 Work queue claim and
retry · M3-03 `decide()` interface and model client · M3-04 Extraction agent ·
M3-05 Skill map and search profile · M3-06 Scoring agent · M3-07 Eval set and
`make evals` · M3-08 Score, reason and gaps in the tracker

## M4: Evidence bank and tailoring

**Goal:** the owner picks a lead and gets an honest tailored CV draft with a coverage
report.

**Exit criteria**
- The Master CV is imported as tagged evidence lines; edits go through the UI or a file.
- The tailoring agent writes only from evidence lines; every bullet carries evidence IDs.
- The coverage report lists covered skills and honest gaps.
- All six guardrail tests from `evidence-bank-guardrails` pass.

**Stories:** M4-01 Evidence bank schema and import · M4-02 Tailoring agent ·
M4-03 Coverage report · M4-04 Guardrail tests · M4-05 Draft review page

## M5: Documents

**Goal:** a draft becomes a ready-to-send pair of one-page PDFs, checked for unsupported
claims.

**Exit criteria**
- Cover letter under 250 words, from evidence only.
- The independent claim checker blocks unsupported or altered sentences.
- One-page PDF export for CV and cover letter; every sent version is kept.

**Stories:** M5-01 Cover letter agent · M5-02 Claim checker · M5-03 PDF export ·
M5-04 Document versions linked to applications

## M6: Full tracker

**Goal:** every application has a status, a history and a next step.

**Exit criteria**
- Statuses from new to offer/rejected/closed/skipped, with a dated history.
- Each application stores a snapshot of the posting as applied to (title, URL, raw payload
  and `content_hash` at that moment). `postings` keeps only the latest payload (ADR-0007),
  so a later edit on the board would otherwise overwrite what was applied to.
- Board view by status; notes and contacts per application.
- Follow-up reminder 7 days after applying with no reply, with a drafted message.
- Skip and skip-with-reason actions stored as feedback.

**Stories:** M6-01 Application status and history · M6-02 Board view · M6-03 Notes and
contacts · M6-04 Follow-up reminders · M6-05 Skip reasons

## M7: Digest and more sources

**Goal:** the morning brief comes to the owner, and coverage grows beyond the ATS boards.

**Exit criteria**
- Morning digest by the chosen channel: top new leads, follow-ups due, postings closing.
- Adapters for at least two remote boards and one Moroccan board whose terms allow it.

**Stories:** M7-01 Choose digest channel (ADR) · M7-02 Morning digest · M7-03 Remote
board adapters · M7-04 Moroccan board adapter · M1-06 Workable adapter (moved from M1)

**M1-06 Workable adapter (backlog).** Pull it in when a company we want posts on Workable.
Its first task is still open: find a public endpoint that actually lists jobs (the widget
endpoint returned 0 jobs for every account in M1-01, Vinted included; see the story).
The Workable accounts in the backlog below are its candidates.

**First new adapter: Moroccan companies.** M1-01 found only one Moroccan company (Jumia)
on Lever, Greenhouse or Ashby; Moroccan employers mostly post elsewhere. Before building,
evaluate Welcome to the Jungle, SmartRecruiters and ReKrute, and pick by how many backlog
companies each covers and whether its terms allow automated reads (source-adapter step 0).

**Backlog: companies not on a supported board** (from M1-01, checked 2026-11-01):
- Morocco: Inyad, Yakeey, YoLa Fresh, YouCan, Freterium, DabaDoc, HPS; Chari and Glovo
  have Workable accounts that list 0 jobs (see M1-06).
- Consulting with Moroccan offices: Capgemini, Globant, SQLI, Devoteam, Xebia, Endava,
  Neoxia, Nimbleways; Zenika and Dial Technologies have Workable accounts (0 jobs).
- Relocation: Odoo, Delivery Hero, Zalando, Revolut, Bolt, Back Market, Ledger, Picnic,
  Bunq, Checkout.com, Aiven; Vinted has a Workable account (0 jobs).
- Remote: Tinybird, Hasura, Redpanda, dbt Labs, Andela, DuckDuckGo; Xata, Qdrant, Fly.io,
  Encore and Hotjar have Workable accounts (0 jobs).
Odoo, Capgemini, Globant, SQLI, HPS, Revolut and Zalando were not probed (own careers
sites or other systems, from memory); confirm when the adapter is chosen.

## M8: Feedback and write-up

**Goal:** the system learns from outcomes, and the project is ready to show.

**Exit criteria**
- Monthly weight review from skip reasons and outcomes is documented and run once.
- Public repository with README, architecture diagram, ADRs and collector benchmarks.
- Threat model reviewed against the finished system.

**Stories:** M8-01 Feedback report and weight tuning · M8-02 Collector benchmarks ·
M8-03 Public README and architecture write-up · M8-04 Threat model review

---

## Change log

| Date | Change | Why |
|---|---|---|
| 2026-11-01 | Roadmap created from the PRD; added M0 walking skeleton; Redis removed (ADR-0001) | Initial plan |
| 2026-11-01 | Local first: M0-05, M1-08 and M1-09 moved to a new milestone **D: Deploy** after M2. M0 and M1 now end with everything working locally. | Hetzner's budget VPS line is sold out; the owner prefers to see the whole pipeline working locally before paying for a server. Deploying after M2 means deploying something already usable. |
| 2026-11-01 | M1-06 Workable adapter moved from M1 to the M7 backlog; M1 now covers Lever, Greenhouse and Ashby. | No active watch-list company uses Workable: the only Workable entry is Yassir, which is excluded and never collected. The public endpoint also listed 0 jobs for every account tried (M1-01), so the adapter's first task is unsolved. All 50 active companies are collected without it. |
| 2026-11-01 | M2 stories written. Liveness: closure is detected from full-board fetches; only manually added postings get the 2-day recheck. | The collector already fetches whole boards, so a per-posting recheck would only duplicate requests. |
| 2026-11-01 | M2-03: hard rules that drop postings replaced by classification plus filter presets; the owner's profile is the `default` preset. New milestone **S: Sharing with friends**, unscheduled, after D. | The owner wants to widen filters at will (seniority included) and to share the tracker with friends in other fields, who need the same data with different filters. Dropping would lose exactly the jobs they want. Sharing changes the trust boundary (ADR-0002), so it gets its own milestone. |
