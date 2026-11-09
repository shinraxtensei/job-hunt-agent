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

M0-05 (server and deploy) moved to **D: Deploy to a server**, now the last milestone.

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

M1-08 (daily schedule and monitoring) and M1-09 (backups) moved to **D: Deploy to a server**: both
only make sense on an always-on server.

**Notes (watch-list upkeep)**
- 2026-11-03: Doctolib's Greenhouse board returned 404 (collector run 10 was `partial`).
  Their careers page now links `jobs.ashbyhq.com/doctolib/…`; the Ashby board lists 150
  jobs. The watch list moved Doctolib to `ashby/doctolib`; the old `greenhouse/doctolib`
  row is deactivated by `companies sync` (ADR-0005), so its postings leave the tracker.

## M2: Rules and a plain tracker (usable from here) ✅ done 2026-11-02

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
M2-06 Plain tracker table (TypeScript) · M2-07 Manual add by link · M2-08 Tracker polish
(added) · M2-09 Classifier country coverage (added)

Order: M2-01 → M2-02 → M2-03 (uses M2-04's `closed` facet, can start before it) →
M2-05 → M2-06. M2-04 and M2-07 can go any time after M2-01.

## L: Whole stack on the Mac (before M3)

**Goal:** the owner runs the whole project on his Mac with one command, the same way it
will run on a server later, and his data survives restarts and can be backed up.

**Exit criteria**
- `make up` starts Postgres, runs migrations and starts the API serving the tracker at
  http://127.0.0.1:8080; every port is bound to 127.0.0.1. `make down` stops it; data
  stays in a named volume.
- `make collect` runs a collection inside the stack, on demand (no daily schedule until D).
- Images are multi-stage, pinned by digest, run as a non-root user with a read-only root
  filesystem where possible: the same images D will deploy.
- `make backup` / `make restore FILE=…` work, and a restore was tested once.
- CI builds the stack and smoke-tests it (healthz, leads).

**Stories:** L-01 Whole stack in Docker Compose on the Mac

## M3: Extraction and scoring ✅ done 2026-11-06

**Goal:** every lead that passes the rules has structured facts, a score, a short
reason, gaps and a CV variant.

**Exit criteria**
- Runs with `make up` on the Mac.
- The Python worker claims work from the Postgres queue (ADR-0001) and survives restarts.
- Extraction returns a validated schema; malformed output retries once, then `needs_review`.
- Scoring follows the PRD weights, held in configuration.
- An eval set of 30–50 labelled postings exists; extraction and scoring meet agreed
  thresholds before merging prompt changes.
- Model cost per lead is logged.

**Stories:** M3-01 Python project and worker skeleton · M3-02 Work queue claim and
retry · M3-03 `decide()` interface and model client · M3-04 Extraction agent ·
M3-05 Skill map and search profile · M3-06 Scoring agent · M3-07 Eval set and
`make evals` · M3-08 Score, reason and gaps in the tracker · M3-09 Location gate (added
during the milestone)

Order: M3-01 → M3-02 → M3-03 → M3-05 → M3-07 → M3-04 → M3-06 → M3-08. M3-07 samples and
labels the eval set before the first extraction prompt; its thresholds are set after
M3-04's first baseline run.

**Wrap-up (2026-11-06).** All exit criteria are met and seen working with `make up`.

- **Delivered:**
  - **Extraction:** all 509 leads the default preset shows are extracted with prompt v2:
    509 done, 0 needs_review.
  - **Scoring:** all 509 are scored in code (PRD weights in `scoring.yaml`). 504 have a
    current reason, gaps and CV variant; 5 need review (1.0%).
  - **Tracker:** best fit first, fit filter with counts, the reason and breakdown in
    the expanded row.
  - **Decisions:** ADR-0017…ADR-0023.
- **Fit levels now:** very strong 4, strong 95, good 143, stretch 107 (97 of them capped
  by the location gate, M3-09), no 160.
- **Cost:**
  - $0.98 of model spend in all since 2026-11-05, evals and the re-runs of this
    milestone included: extraction $0.52, reasons $0.22, CV variant $0.13, eval
    pre-fill $0.03.
  - Running cost per new lead: about $0.0007 to extract, plus $0.0002 for the reason and
    CV variant. Logged per call in `model_calls`. The daily budget was never reached.
- **Extraction evals (prompt v2):**

  | Field | All 36 | Held-out 10 |
  |---|---|---|
  | Required skills, F1 (precision) | 72.3% (77.6%) | 66.2% (69.0%) |
  | Optional skills, F1 | 73.3% | 70% |
  | Languages, remote policy | 94.4% | 90% |
  | Years, seniority, visa, salary | 100% | 100% |

  Injection checks pass 4/4 on every run. The thresholds are regression gates
  (all-36 score minus 0.03), and `make evals-replay` checks them in CI.
- **Fit agreement with the owner's blind labels (all 36; held-out was used for the
  first band fit, so it is no longer unseen):**
  - before the location gate: exact 55.6%, within one level 88.9%;
  - with it: exact 47.2%, within one level 91.7%.

  The gate costs exact agreement on three leads the owner labelled good or strong in
  countries they can't work from (US, Canada). It moves "no" leads such as GitLab India
  off "strong".
- **Reasons:** prompt v3; 1.0% need review. The rule that failed is named in the error;
  most often it is the check that a missing or learning skill is never claimed as
  experience. The reasons never see posting text.

**What M4 needs from the owner:**
- **The Master CV**, tagged by variant (BE, PAY, DIST, FS, AI, CONS). It is imported as
  evidence lines (M4-01), and every tailored bullet must cite one.
- **Evidence notes** for the 9 skills set to familiar for lack of evidence
  (Observability, IaC, Ubuntu, Agile, Debian, Bash, Virtualization, Grafana, DevOps),
  if any of them should be proven. Otherwise they stay familiar, and the CV never
  presents them as experience.
- **A CV-variant label** for each of the 36 eval postings, so the variant choice can be
  measured before tailoring depends on it (see the follow-up below).
- **One-page CV layout:** the template to fill, or approval of the first draft's
  layout (M5 makes the PDFs).
- Review and approval of the M4 stories before work starts.

**Follow-ups from M3 (not scheduled yet):**
- **Junk locations** such as "N/A" (7 shown leads) appear in the location detail and
  the reasons as written. They should be dropped at normalisation (M2's classifier).
- **The location factor reads the canonical posting's own facets,** not the group
  member that matched the preset (ADR-0023 revisit). The detail names the matched
  location, but the value comes from the canonical job.

**Follow-ups from M3-06 (not scheduled yet):**
- **"Stretch" as a flag, not a band.** The calibrated stretch band is 1 point wide
  (32–33). Make "stretch" a flag on the lead instead: the job's seniority or minimum
  years is above the profile's. Keep four fit levels.
- **The CV-variant choice is skewed.** In the 509-lead run, `decide()` chose DIST for
  158 (173 after the reasons were regenerated). Label the variant on the eval set and evaluate it in M4, before tailoring
  depends on it.

## M4: Evidence bank and tailoring ✅ done 2026-11-08

**Goal:** the owner picks a lead and gets an honest tailored CV draft with a coverage
report.

**Exit criteria**
- Runs with `make up` on the Mac.
- The Master CV is imported as tagged evidence lines; edits go through the UI or a file.
- The tailoring agent writes only from evidence lines; every bullet carries evidence IDs.
- The coverage report lists covered skills and honest gaps.
- All six guardrail tests from `evidence-bank-guardrails` pass.

**Stories:** M4-01 Evidence bank · M4-02 Tailoring agent · M4-03 Coverage report ·
M4-04 Draft validator and the guardrail tests · M4-05 Draft review page · M4-06 Measure
the CV-variant choice (added from the M3 follow-ups)

Order: M4-01 → M4-06 (in parallel) → M4-03 → M4-04 → M4-02 → M4-05.
- The validator (M4-04) comes before the tailoring agent, so the guardrail tests exist
  before the code they guard.
- Coverage (M4-03) is the agent's input, so it comes before both.
- The model claim checker stays in M5 (M5-02). M4-04 is the deterministic layer under
  it.

**Owner input** (formats in `docs/stories/M4-owner-input.md`):

| # | What | File | Needed by |
|---|---|---|---|
| OI-1 | Master CV as checked evidence lines | `data/profile/evidence.yaml` | M4-01, blocks real drafts |
| OI-2 | Evidence for the 9 skills set to familiar | `skill_map.yaml` | optional |
| OI-3 | CV-variant label per eval posting | `agents/evals/variants.yaml` | M4-06 |
| OI-4 | One-page layout | `data/profile/cv_layout.yaml` | M4-02; defaults until then |
| OI-5 | Story review | the stories PR | all |

**Wrap-up (2026-11-08).** All exit criteria are met and seen working with `make up`.

- **Delivered:**
  - **Evidence bank:** the owner's Master CV as 29 checked lines (`evidence.yaml`),
    validated and synced into versioned tables; removed lines are retired, never
    deleted. Edits go through the file (ADR-0024); the UI shows lines, not edits.
  - **Coverage:** covered, listed (familiar) and gaps per lead, in code; instruction-like
    skill names flagged as suspicious.
  - **Validator:** the six guardrail tests pass, plus scope-inflation and
    familiar-in-bullet checks. Any violation blocks the draft.
  - **Tailoring:** selection in code, the model sees only selected lines and skill
    names, facts come from the bank. `make evals-tailor`: 21/21 good-fit drafts
    ready_for_review, 0 injection failures, ~3.9 s and ~$0.0007 per draft.
  - **Review page:** draft with evidence per bullet, findings under their sentence,
    coverage, approve/discard (nothing sent).
  - **CV variant:** a code rule from the score; 75% agreement with the role-family proxy
    (was 52.8% for `decide()`); DIST 173 → 122 leads.
- **Spend:** $1.01 model spend in all so far (tailoring $0.03).
- **Skipped / open:** OI-3 variant labels (proxy used), OI-2 evidence for the 9
  familiar skills, OI-4 layout (defaults). Editing bullets by hand stays out (M5 if
  needed).

## M5: Documents ✅ done 2026-11-08

**Goal:** a draft becomes a ready-to-send pair of one-page PDFs, checked for unsupported
claims.

**Exit criteria**
- Runs with `make up` on the Mac.
- Cover letter under 250 words, from evidence only.
- The independent claim checker blocks unsupported or altered sentences.
- One-page PDF export for CV and cover letter; every sent version is kept.

**Stories:** M5-01 Cover letter agent · M5-02 Claim checker · M5-03 PDF export ·
M5-04 Document versions (the link to applications moved to M6, which creates them)

Order: M5-01 → M5-02 → M5-04 → M5-03. Owner input: `docs/stories/M5-owner-input.md`.

**Wrap-up (2026-11-08).** All exit criteria are met and seen working with `make up`.
One of the owner's real drafts was approved, and its PDFs checked: one A4 page each,
every sentence in the selectable text, and ’ and — intact.

- **Delivered:**
  - **Cover letter:** under 250 words, made with the CV in one draft. Code writes the
    greeting, the opening (job title and company), the sign-off and the name. The
    letter model gets the role facts but never the posting. Letter-only `letter` lines
    in `evidence.yaml`.
  - **Claim checker:** an independent pass over every sentence with its cited lines,
    with thinking on. It catches 25/25 planted cases. Real catches the code validator
    missed: "Built" from "Helped build", invented closing wishes, a product named as
    the employer.
  - **Documents:** immutable rows (bytes and SHA-256), marked sent once by the owner
    (ADR-0026).
  - **PDF export:** fpdf2 with vendored Noto Sans (ADR-0027). Text only; no image is
    ever parsed (ruff ban plus a test); `make vuln` scans the Python lockfile, and its
    first run caught a vulnerable pypdf pin.
- **Results:**
  - `make evals-tailor` with the checker: 17/21 ready, 0 injection failures, ~$0.002
    and ~12 s per draft.
  - Live, lead 55: 2 drafts blocked by the checker, the third ready. A block costs one
    more click.
- **Method:** every M5 test was proved by breaking the code it guards (the tables are
  in #53 to #56). Two checks turned out to be dead code or untested, and were fixed.
- **Spend:** $1.46 in model calls so far.
- **Open:**
  - the checker's verdicts vary between runs (a re-check flags ~2 of 17 ready drafts);
  - no hand edits before export (OI-9);
  - letters are English only (OI-7; French evidence for M7-04).

## M6: Full tracker ✅ done 2026-11-08

**Goal:** every application has a status, a history and a next step.

**Exit criteria**
- Runs with `make up` on the Mac.
- Statuses from new to offer/rejected/closed/skipped, with a dated history.
- Each application stores a snapshot of the posting as applied to (title, URL, raw payload
  and `content_hash` at that moment). `postings` keeps only the latest payload (ADR-0007),
  so a later edit on the board would otherwise overwrite what was applied to.
- Each application links the document versions sent with it (M5-04's `documents`).
- Board view by status; notes and contacts per application.
- Follow-up reminder 7 days after applying with no reply, with a drafted message.
- Skip and skip-with-reason actions stored as feedback.

**Stories:** M6-01 Application status and history · M6-02 Board view · M6-03 Notes and
contacts · M6-04 Follow-up reminders · M6-05 Skip reasons · M6-06 Company merge (from the
M2-07 note below)

Order: M6-01 → M6-05 → M6-03 → M6-04 → M6-02 → M6-06; the board can slip (PRD cut
line). Owner input: `docs/stories/M6-owner-input.md`.

**Wrap-up (2026-11-08).** All exit criteria are met; the board was built as well. Every
M6 test was proved by breaking the code it guards (#58 to #63).

- **Delivered:**
  - **Applications (ADR-0028):** one per lead group, with an append-only status
    history; `drafted` is set automatically on approve; a snapshot of the posting is
    kept when first applied; sent documents are linked.
  - **Skips:** one click with Undo, a reason from your list, and the score at that
    moment (`/api/skips` for M8).
  - **Notes and contacts:** kept only in the database. A test plants a note and a
    contact in a failing request and checks the log; another checks no agent code reads
    them.
  - **Follow-ups:** due 7 days after `applied`, with your template filled in by code;
    copy, Done, Snooze 3 or 7 days.
  - **Board:** OI-13's columns, moved by drag or menu, one column on phones.
  - **Company merge:** `collector merge-company`.
- **Found by the checks:**
  - a test that couldn't see an overwritten `applied_at`;
  - follow-up history written with the wrong clock;
  - a board that scrolled sideways at 1280 px;
  - dark-mode contrast on native controls (CI's Linux browser);
  - two pieces of dead code, removed.
- **Live:** the tracker, board and follow-ups run on the local stack. Statuses on your
  real leads are yours to set: none were set on your behalf.
- **Open:**
  - the board's Done column sets `closed` on drop (the menu picks rejected or skipped);
  - reapplying to the same group a year later needs a second application
    (ADR-0028's "revisit when").

## v1 complete (2026-11-08)

M0 to M6 and L are done: collection, scoring, CV and cover-letter drafts from the
evidence bank with a claim checker, one-page PDFs, and the full tracker all run locally
with `make up`. The README's "How to use it daily" is the owner's guide.

Remaining milestones, in order:
- **M7:** digest and more sources;
- **M8:** feedback and write-up;
- **D:** deploy to a server, last as planned;
- **S:** sharing (unscheduled).

Model spend for all of v1's building and evals: about $1.50.

**Notes**
- Company merge (from M2-07, ADR-0015): a board-less company typed with `collector add
  --company` and the same company later added to the watch list are two `companies` rows,
  so their leads don't group together. Merging them (and moving the manual postings) is M6
  work.
- Closing manual non-board postings is the owner's action here; they are never rechecked.

## M7: Today view and more sources ✅ done 2026-11-09

**Goal:** opening the tracker shows what changed since the last visit, and coverage grows
to Moroccan employers and remote boards, from sources whose terms allow it.

**Exit criteria**
- Runs with `make up` on the Mac.
- A "Today" view: new leads since the last visit (best first), follow-ups due, postings
  closed since the last visit.
- Moroccan employers first: every source checked with the source-adapter step 0
  (`docs/research/m7-sources.md`); adapters only where the terms allow automated reads.
- At least two remote boards whose terms allow it.

**Stories:** M7-01 Today view · M7-02 Moroccan employers: Personio feed and French drafts ·
M7-03 Remote boards: Remotive and RemoteOK · M1-06 Workable adapter (backlog)

Order: M7-01 → M7-02 → M7-03. Owner input: `docs/stories/M7-owner-input.md`.

**Done (2026-11-09).** M7-00 redesign (#70), M7-01 Today view (#71), M7-02 Personio feed
and French drafts (#72, #73), M7-03 Remotive and Remote OK (#74, #75), ADR-0029.
- Today shows new leads since the last visit, follow-ups due and closed postings.
- Inyad comes in through Personio. French postings get French drafts once the owner
  writes `text_fr` lines (OI-21); until then they get English drafts that say why.
- Remotive and Remote OK add about 70 remote jobs of companies outside the watch list;
  the default preset shows about 25.
- Still open: OI-18 SmartRecruiters (default no), OI-19 other remote boards' terms,
  M1-06 Workable (backlog).

**Changed 2026-11-09 (owner):** no push digest before D. Collection is on demand on the
Mac, so a morning email or Telegram message has nothing new to say on its own. The
digest and its channel ADR moved to D. The Today view replaces them here.

**Step 0 result:** no Moroccan job board allows automated reads (ReKrute and Welcome to
the Jungle forbid it in their terms; Emploi.ma blocks automated fetches). Moroccan
employers come through their own career feeds instead: Personio (Inyad) now,
SmartRecruiters (Devoteam, Alten, Delivery Hero) if the owner accepts it (OI-18).

**M1-06 Workable adapter (backlog).** Pull it in when a company we want posts on Workable.
Its first task is still open: find a public endpoint that actually lists jobs (the widget
endpoint returned 0 jobs for every account in M1-01, Vinted included; see the story).
The Workable accounts in the backlog below are its candidates.

**First new adapter: Moroccan companies.** *(Evaluated 2026-11-09: see
`docs/research/m7-sources.md` and M7-02.)* M1-01 found only one Moroccan company (Jumia)
on Lever, Greenhouse or Ashby; Moroccan employers mostly post elsewhere. Before building,
evaluate Welcome to the Jungle, SmartRecruiters and ReKrute, and pick by how many backlog
companies each covers and whether its terms allow automated reads (source-adapter step 0).

**French letters (M7-04 note, owner 2026-11-08).** Letters are English-only (M5 OI-7).
French postings from a Moroccan board will need French evidence lines, written and
checked by the owner. Evidence is never machine-translated.

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
- Runs with `make up` on the Mac.
- Monthly weight review from skip reasons and outcomes is documented and run once.
- Public repository with README, architecture diagram, ADRs and collector benchmarks.
- Threat model reviewed against the finished system.

**Stories:** M8-01 Feedback report and weight tuning · M8-02 Collector benchmarks ·
M8-03 Public README and architecture write-up · M8-04 Threat model review

## D: Deploy to a server (last)

**Goal:** the finished stack, already running with `make up` on the Mac (L–M8), runs on an
always-on server only the owner can reach, collects every morning on its own, and its
data is backed up off the server. Until then, collection runs on demand (`make collect`).

**Exit criteria**
- A push to `master` deploys automatically; the server is reachable only over Tailscale.
- The collector runs daily on the server; runs are recorded and a missed run alerts the owner.
- A nightly encrypted backup exists off the server, and a restore was tested once.
- Access is decided: ADR-0002 (no built-in auth, Tailscale only) is Accepted or replaced.
- The server runs the same images as the Mac (L-01).

**Stories:** M0-05 Server, Tailscale and deploy · M1-08 Daily schedule and run monitoring ·
M1-09 Nightly backup and tested restore (story IDs kept from their original milestones) ·
D-01 Choose digest channel (ADR) · D-02 Morning digest (both moved from M7 on 2026-11-09:
a pushed digest needs the daily schedule)

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

---

## Change log

| Date | Change | Why |
|---|---|---|
| 2026-11-01 | Roadmap created from the PRD; added M0 walking skeleton; Redis removed (ADR-0001) | Initial plan |
| 2026-11-01 | Local first: M0-05, M1-08 and M1-09 moved to a new milestone **D: Deploy** after M2. M0 and M1 now end with everything working locally. | Hetzner's budget VPS line is sold out; the owner prefers to see the whole pipeline working locally before paying for a server. Deploying after M2 means deploying something already usable. |
| 2026-11-01 | M1-06 Workable adapter moved from M1 to the M7 backlog; M1 now covers Lever, Greenhouse and Ashby. | No active watch-list company uses Workable: the only Workable entry is Yassir, which is excluded and never collected. The public endpoint also listed 0 jobs for every account tried (M1-01), so the adapter's first task is unsolved. All 50 active companies are collected without it. |
| 2026-11-01 | M2 stories written. Liveness: closure is detected from full-board fetches; only manually added postings get the 2-day recheck. | The collector already fetches whole boards, so a per-posting recheck would only duplicate requests. |
| 2026-11-01 | M2-03: hard rules that drop postings replaced by classification plus filter presets; the owner's profile is the `default` preset. New milestone **S: Sharing with friends**, unscheduled, after D. | The owner wants to widen filters at will (seniority included) and to share the tracker with friends in other fields, who need the same data with different filters. Dropping would lose exactly the jobs they want. Sharing changes the trust boundary (ADR-0002), so it gets its own milestone. |
| 2026-11-02 | **M2 complete.** M2-01…M2-07 done, plus two stories added during the milestone: M2-08 Tracker polish (matched location, labels) and M2-09 Classifier country coverage (all ISO 3166-1 countries, `latam`). Decisions: ADR-0008…ADR-0015. On the local data: 5,902 jobs in 4,486 duplicate groups; the default preset shows 508 and hides 3,978, each with its reasons; closed postings detected from full-board fetches; jobs can be added by link (`collector add`), with board links fetched only through the boards' own APIs. The tracker page (Vite + React, embedded in the API binary, same-origin CSP) passes axe and AA contrast at 375 and 1280 px in both themes. Held back: TypeScript 7 (Dependabot #26) until typescript-eslint supports it (typescript-eslint#10940). Next: **D: Deploy**. | Every M2 exit criterion is met and seen working locally. TypeScript 7 passes `tsc`, Vite and Vitest, but typescript-eslint 8.72.1 (latest) requires TypeScript < 6.1 and refuses to run, so `npm ci` and the lint step fail. |
| 2026-11-03 | **D: Deploy to a server** moved from after M2 to the end (after M8); M0-05, M1-08 and M1-09 stay in it, and **S: Sharing** stays after D (it needs a server). New milestone **L: Whole stack on the Mac** before M3 (story L-01). Every milestone from M3 on must run with `make up` on the Mac. ADR-0002 stays Proposed; it is decided in D. | Owner's choice: the whole project runs on the Mac first and the server comes last. Nothing needs daily automatic collection until then: collection runs on demand with `make collect` (`collector run` inside the stack). Running the same images locally now means D only has to move them. |
| 2026-11-06 | M3-06 follow-ups recorded under M3: stretch as a seniority/years flag instead of a 1-point band; CV-variant choice to be evaluated with labels in M4. | Owner's review of #42: the calibrated stretch band is 1 point wide, and `decide()` chose DIST for 158 of 509 leads. |
| 2026-11-06 | **M3 complete.** M3-01…M3-09; M3-09 Location gate added during the milestone (owner's review of #43). 509 shown leads extracted and scored; $0.98 model spend in all; extraction gates and fit agreement in the M3 wrap-up. Next: **M4**. | Every M3 exit criterion is met and seen working locally. |
| 2026-11-06 | M4 stories written: M4-01…M4-05 renamed to match their scope (Guardrail tests → Draft validator and the guardrail tests), M4-06 added (CV-variant measurement, from the M3 follow-ups), order set, owner input listed with formats. | Owner's direction after M3; the validator comes before the agent so the guardrail tests exist first. |
| 2026-11-08 | **M4 complete.** M4-01…M4-06 (#47…#51). ADR-0024, ADR-0025. Owner's evidence bank loaded; drafts validated before shown; review page in the tracker. OI-3 skipped (role-family proxy), OI-2/OI-4 optional. Next: **M5**. | Every M4 exit criterion is met and seen working locally. |
| 2026-11-08 | M5 stories written; linking documents to applications moved from M5-04 to M6. | Applications are created in M6. |
| 2026-11-08 | **M5 complete.** M5-01…M5-04 (#53…#56); ADR-0026, ADR-0027. M6 stories written, M6-06 Company merge added from the M2-07 note. Next: **M6**. | Every M5 exit criterion is met and seen working locally. |
| 2026-11-08 | **M6 complete; v1 complete.** M6-01…M6-06 (#58…#63), ADR-0028. README: "How to use it daily". Next: **M7**, then M8, then D. | Every M6 exit criterion is met; the owner asked to mark v1 complete when M6 is done. |
| 2026-11-09 | M7 reshaped: the digest and its channel ADR moved to D; M7 is a Today view, Moroccan employers (Personio feed, French drafts) and remote boards (Remotive, RemoteOK). Step 0 research in `docs/research/m7-sources.md`. | Owner: no push digest before the server collects on its own. Step 0 found no Moroccan job board whose terms allow automated reads. |
| 2026-11-09 | **M7 complete.** M7-00…M7-03 (#70…#75), ADR-0029. Next: **M8**, then D. | Every M7 exit criterion is met: Today view, Moroccan employers through the one source whose terms allow it, two remote boards. |
