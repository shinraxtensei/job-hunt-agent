# M3-07: Eval set and `make evals`

- **Status:** Done
- **Milestone:** M3 — Extraction and scoring
- **Linear:** [HAM-46](https://linear.app/hamid-corp/issue/HAM-46/m3-07-eval-set-and-make-evals)
- **Estimate:** 1 day

## Story

As the job seeker, I want a labelled set of real postings and one command that measures
extraction and scoring against it, so that no prompt or model change makes the agents
worse without me knowing.

## Context

CLAUDE.md: "changes to prompts or models must not lower eval scores (`make evals`)". The
roadmap's M3 exit criteria: 30–50 labelled postings, agreed thresholds before merging
prompt changes. Order: this story comes **before** M3-04, so the extraction prompt is
measured from its first version.

## Acceptance criteria

1. **Given** 30–50 postings sampled from the local data (EN and FR, several role
   families, including the prompt-injection fixtures from M3-04 and M3-06), sampled and
   labelled **before the first extraction prompt is written**
   **When** the owner labels them
   **Then** the **fit level is labelled blind**, before any model score exists or is
   shown; the extraction fields may be **pre-filled by a model and corrected by the
   owner**, and each label records which fields he changed (so the pre-fill's own error
   rate is visible). Stored under `agents/evals/` with their source posting ids; posting
   text kept local (git-ignored, backed up by M3-05 AC5), labels committed.
2. **Given** `make evals`
   **When** it runs against the real model
   **Then** it reports per-field extraction accuracy (skill lists as precision, recall and
   F1 after name folding and skill-map aliases), fit-level agreement and the
   injection checks, plus the run's total cost, and fails below the agreed thresholds.
   The thresholds are set **after M3-04's first baseline run** and recorded with it.
3. **Given** CI
   **When** it runs
   **Then** a replay mode re-scores recorded model outputs (no network, no cost), so a
   change to scoring code or schemas that breaks the evals fails CI.
4. **Given** a PR changing a prompt or a model
   **When** it is opened
   **Then** it includes the `make evals` report before and after.

## Labelling guide (extraction fields)

Strictly from the posting text; nothing inferred. Agreed with the owner on 2026-11-05.

- **required_skills**: what the posting says the candidate must have, plus the
  technologies the duties say the person will use day to day (e.g. "Build TypeScript +
  GraphQL APIs" → TypeScript, GraphQL). Tools **and** named domains (AML, payments,
  vector search).
- **optional_skills**: items marked nice-to-have / bonus / a plus / preferred, **and**
  the technologies of a "team stack / our app uses / we build on" section (a description
  of the team's tools rather than of the candidate or the duties), unless the
  requirements or duties name them too, which makes them required (added 2026-11-05,
  M3-04 review).
- **Skill names are canonical and matchable**: the kind of name a skill map holds
  (a technology, tool or named domain). No soft skills and no activity phrases
  ("incremental refactoring", "research leadership"); rewrite to a real skill name
  ("System design") or drop. An example after "such as" / "e.g." is not itself a
  requirement; the thing it exemplifies is.
- **min_years**: the number stated, else empty. **seniority**: only if stated or the
  title says it ("Intermediate" → mid, "Manager" leading a team → lead).
- **visa_sponsorship**: yes/no only if stated (a citizenship requirement is "no"), else
  unknown. **salary**: only if stated. **languages**: only those stated as required.
- **remote_policy**: as stated. "Office-first" → onsite; remote only when the text says
  remote / work from home / hire anywhere.

Each label records `labelled_by` (`owner` from the CLI, `claude-code` when Claude Code
labelled it under these rules for the owner's spot-check) and `changed_fields` against
the pre-fill (lists compared as sets of folded names). The CLI saves every answer as it
is given (`draft`), so quitting mid-posting loses nothing.

**Scoring skill lists**: precision, recall and F1 per item, averaged, after folding names
like M3-05 (case, accents, dots and dashes), resolving them through the committed
tech-alias table (`agents/src/jobhunt_agents/tech_aliases.yaml`: GCP = Google Cloud
Platform, K8s = Kubernetes, ...; never tuned against held-out) and then through the skill
map's names and aliases when `PROFILE_DIR` has one (the report says which; CI has no personal
skill map, so it folds only). Exact-string matching would punish wording, not mistakes.

## Out of scope

- Automatic weight tuning (M8). Large eval sets.

## Security notes

Eval runs spend money: capped by the M3-03 daily budget and printed. Posting texts are
untrusted; the eval report never prints model output unescaped.
