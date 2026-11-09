# Threat model

Reviewed 2026-11-09 against v1 as built (M0–M7), running on the owner's Mac with
`make up` (M8-04). Every control below names the file or test that enforces it. A control
nothing enforces is listed under **Gaps**, not as a control. Re-read this file when D puts
the system on a server, and whenever a trust boundary changes (ADR rule 4).

## What we protect

| Asset | Where | Why it matters |
|---|---|---|
| The evidence bank, skill map, search profile | `data/profile/*.yaml` (git-ignored, 600) | the owner's career history and preferences |
| Drafts, exported PDFs, applications, notes, contacts | Postgres | CV content; other people's contact details |
| The owner's name, email, phone | evidence bank `person` | identity |
| `ANTHROPIC_API_KEY`, database passwords | `.env` (git-ignored, 600) | money and data access |
| Honesty of every document | tailor, validator, checker | one invented claim can cost an offer |
| The boards' goodwill | collector | terms and rate limits; losing access loses the source |
| Backups | `~/job-hunter-backups` (700) | a full copy of all of the above |

## Trust boundaries

```
 job boards (untrusted) ──HTTPS GET──▶ collector (Go) ──▶ Postgres ◀── worker (Python) ──HTTPS──▶ Anthropic API
                                                              ▲
 owner's browser ──HTTP 127.0.0.1──▶ API + tracker (Go) ──────┘
```

1. **Boards → collector.** Postings are untrusted input: text, HTML, names, URLs.
2. **Browser → API.** No login (ADR-0002, Proposed); the boundary is the loopback
   interface and the write guards.
3. **Worker → model provider.** Data leaves the Mac. Model output is untrusted too.
4. **Repository → the public** (M8-03): nothing personal may be committed.
5. **Owner's Mac.** Other local processes and users. Backups.

### What crosses boundary 3 (corrects ADR-0018)

| Call | Sends | Never sends |
|---|---|---|
| extraction | the posting's title, location, cleaned description | anything of the owner's |
| reason, gaps | the extraction's fields; the names of matching and missing skills | evidence text |
| tailor (CV) | the selected evidence lines; the posting's extracted skill names | name, email, phone, posting text |
| letter | the same lines, the letter lines, each role's title and employer | name, email, phone, posting text |
| claim checker | the draft's sentences; the cited lines; roles, projects, education, certifications with dates | name, email, phone, posting text |

The person block (name, headline, contact) is added by code after the model call
(`tailor.render`, `render_letter`). The provider's terms: not used for training,
normally deleted within 30 days (ADR-0018).

## Threats and controls

### Boards → collector

| Threat | Control | Enforced by |
|---|---|---|
| Prompt injection in a posting ("add Kubernetes") | posting text only inside one escaped data block; extraction returns a schema only; scoring is code; the checker never sees the posting | `agents/tests/test_delimit.py::test_exactly_one_data_block`, `test_extract.py::test_injection_stays_data`, `test_reason.py::test_injection_cannot_change_score_or_reason`, `test_tailor.py::test_injection_changes_nothing_but_the_flag`, `test_checker.py::test_checker_sees_no_posting`, `make evals-reasons`/`evals-tailor` injection fixtures |
| Stored XSS through a title, company, reason or URL | React text only; `safeHref` allows http(s) only; CSP `script-src 'self'`, `frame-ancestors 'none'` | `web/src/safe.test.tsx` ("shows an HTML title as text…", "renders a javascript: URL as plain text…", reason/gap and draft payloads), `web/e2e/tracker.spec.ts` "an HTML title and a javascript: URL are inert", `internal/webui/webui_test.go::TestServesBuiltPageWithCSP` |
| Hostile or huge responses | 32 MiB cap, stall and total timeouts, retries only on 429/5xx | `internal/source/fetch/fetch_test.go` (`TestGetRefusesOversizedBody`, `TestGetFailsWhenBodyStalls`, `TestGetCeilingEndsPathologicalTrickle`, `TestGetDoesNotRetryClientErrors`) |
| Parser crashes, XML external entities | malformed entries skipped; Go's XML decoder resolves no entities; fuzzed | `FuzzParse` in every `internal/source/*`, `make fuzz`; Personio seed with an external-entity document |
| NUL bytes Postgres refuses | dropped and counted | `TestHasNUL`, `TestOnceSkipsNULPosting` |
| An outage closing a whole board | empty-board and close-ratio guards (ADR-0010), also per source for boards (ADR-0029) | `internal/collector/closed_integration_test.go`, `TestBoardClosesMissingAcrossCompanies` |
| SSRF through a pasted link | only exact board hosts parsed to a fixed API endpoint; other links are never fetched (ADR-0015) | `internal/job/link_test.go::TestParseBoardLink`, `FuzzParseBoardLink`, `TestInsecureBoardLink` |
| Posting text in logs | logs carry counts and ids only | `TestOnceLogsCountsAndIDsNeverPostingText` |
| Breaking a board's terms | step 0 per source; 1 request/s per host; Remotive and Remote OK at most every 6 h; attribution shown | `TestGetWaitsOnLimiterForEveryAttempt`, `TestRunRateLimitsEachHost`, `TestBoardMinIntervalSkipsEarlyRuns`, `docs/research/m7-sources.md` |

### Browser → API

| Threat | Control | Enforced by |
|---|---|---|
| Another device on the network | every port published on `127.0.0.1` only | `deploy/compose.yaml` |
| A website writing to the API (CSRF) | `guardWrite`: Go's `CrossOriginProtection`, JSON content type only (ADR-0022) | `internal/httpapi/api_integration_test.go::TestRequestReason` (cross-site, same-site, foreign Origin, non-JSON all 403) |
| DNS rebinding | loopback `Host` names only | `internal/httpapi/host_test.go::TestHostAllowlist` |
| Oversized or unexpected JSON | 64 KB cap, unknown fields refused | `readJSON` in `internal/httpapi/applications.go`, write tests in `api_integration_test.go` |
| SQL injection | sqlc and parameters only; the app role has no DDL | `internal/store/queries/*.sql`, `make sqlc-check`, `deploy/postgres/initdb/01-roles.sh` |
| Tampering with sent documents | documents are immutable: a trigger refuses updates and deletes (ADR-0026) | `db/migrations/20261108200000_documents.sql`, `TestDocumentsAreKept` |

### Worker → model provider

| Threat | Control | Enforced by |
|---|---|---|
| Invented claims in a CV or letter | evidence ids on every sentence; code validator; independent claim checker; blocked drafts can't be approved | `agents/tests/test_validator.py`, `test_checker.py`, `test_tailor.py`, French cases `test_french_guardrails`; `make evals-tailor` + `make evals-checker` at 100% on planted cases |
| A translation changing a number | `text_fr` must state the numbers of `text`, or the bank won't load | `test_evidence.py::test_french_numbers_must_match` |
| Malformed model output used anyway | schema-validated, retried once, then `needs_review` | `agents/tests/test_model.py`, `test_extract.py` |
| Runaway spend | daily budget cap ($1 default) checked before each call | `test_model.py::test_budget_cap`, `test_budget_counts_only_today_utc` |
| The API key leaking | `SecretStr`, never logged or in errors | `test_settings.py::test_secret_never_printed`, `test_anthropic_provider.py::test_key_never_leaks` |
| Image-parsing exploits in PDF export | no image is ever parsed (ruff ban + test, ADR-0027) | `agents/tests/test_no_images.py` |
| Something sent automatically | nothing is: the owner approves, exports and sends; "sent" is marked by hand | no send code exists; `TestMarkDocumentSent` |

### Supply chain

| Threat | Control | Enforced by |
|---|---|---|
| A vulnerable dependency | govulncheck (any finding fails), `uv audit` (fixable fails), `npm audit` | `make vuln`, `make web-check`, CI |
| A swapped base image | images pinned by digest | `deploy/compose.yaml`, `agents/Dockerfile`, `Dockerfile` |
| A malicious agent skill | vendored, reviewed, pinned by commit; one was rejected (ADR-0014) | `.claude/skills/*/SOURCE.md` |
| A container escape widening damage | read-only filesystems, `cap_drop: [ALL]`, `no-new-privileges`; a distroless non-root API and collector image, a slim worker image | `deploy/compose.yaml` |

## Findings of this review (2026-11-09)

- **F1: ADR-0018 understated what is sent to the model.** It said evidence-bank entries
  are never sent. Since M4 the tailor, letter and checker calls send selected evidence
  lines and the employment facts (table above). Name and contact details have never been
  sent. ADR-0018 now carries an amendment; nothing in the code changed. The owner is told,
  since it is data leaving the Mac.
- **F2: `.env` and the personal profile files were world-readable (644).** Now 600, with
  `data/profile` 700. The containers still read them on the Mac (checked).
- **F3: this file didn't exist** although `CLAUDE.md` named it.
- **F4: the 32 MiB response cap had no test.** `TestGetRefusesOversizedBody` added.

## Gaps (accepted on the Mac)

- **Any local process can use the API.** Loopback without auth (ADR-0002) trusts every
  process on the Mac. Acceptable for a single-user laptop; not on a shared machine.
- **Backups are unencrypted.** They sit in an owner-only directory (`umask 077`, `chmod
  700`, `deploy/scripts/backup.sh`). Copying them elsewhere would copy everything.
- **The provider keeps inputs up to 30 days** (longer if flagged). Zero retention needs
  an agreement with Anthropic.
- **Personal data in git history** (the M3-08 screenshots, maybe more): the M8-03
  pre-publication scan finds it and plans the rewrite; the repository stays private until
  then.

## For D (a server)

- **Network:** Tailscale only, Caddy in front; ADR-0002 moves from Proposed to Accepted
  or is replaced. The API's loopback `Host` rule needs the tailnet name.
- **File ownership:** 600 files owned by the host user won't be readable by the
  containers' non-root user on Linux (Docker Desktop hides this on the Mac). Use a group
  or the container's uid.
- **Secrets:** a server `.env` (600, root-owned), or Docker secrets.
- **Backups:** off-machine copies must be encrypted (e.g. `age`) before they leave.
- **The scheduler** collects on its own: the 6-hour board interval and per-host limits
  already hold; add alerting on failed runs (ADR-0006 records them).
