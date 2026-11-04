# M3-07: Eval set and `make evals`

- **Status:** Ready
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
   **Then** it reports per-field extraction accuracy, fit-level agreement and the
   injection checks, plus the run's total cost, and fails below the agreed thresholds.
   The thresholds are set **after M3-04's first baseline run** and recorded with it.
3. **Given** CI
   **When** it runs
   **Then** a replay mode re-scores recorded model outputs (no network, no cost), so a
   change to scoring code or schemas that breaks the evals fails CI.
4. **Given** a PR changing a prompt or a model
   **When** it is opened
   **Then** it includes the `make evals` report before and after.

## Out of scope

- Automatic weight tuning (M8). Large eval sets.

## Security notes

Eval runs spend money: capped by the M3-03 daily budget and printed. Posting texts are
untrusted; the eval report never prints model output unescaped.
