# Job-Hunt Agent: Product Requirements (summary)

Full, editable PRD: https://claude.ai/artifact/ETPNX5BhXxhyvnJQoZkXgW
This file is the in-repo summary used as context. When the two disagree, the linked PRD
wins and this file gets updated.

## Summary

Finds new job postings every day, scores each against the owner's profile, prepares a
tailored CV and cover letter for the good ones, and tracks every application to its
outcome. The owner reviews and approves; the agent searches, reads and drafts.
Version 1 has one user. It is also a learning project: Go for collection, Python for agents.

## Origin

Automates a manual workflow: ranked title list (EN + FR) → search profile; ATS keyword
clusters → skill map (proven / familiar / learning); tagged Master CV (BE, PAY, DIST, FS,
AI, CONS) → evidence bank; manual leads spreadsheet → tracker.

## Goals

- Matching postings appear within 24 hours, with no manual action.
- Every lead has a fit score and a reason checkable in 10 seconds.
- Tailored one-page CV and cover letter ready within 2 minutes of asking.
- Every application has a status and a history, from found to outcome.
- No CV statement that is not in the evidence bank.

## Non-goals (v1)

Automatic submission; logging in to LinkedIn or any site; interview prep or negotiation;
multiple users, billing, public sign-up; a mobile app (the web UI must work on a phone).

## Pipeline

1. **Collect** (Go): Lever, Greenhouse, Ashby, Workable first; daily watch list.
2. **Normalise** (Go): one common job record.
3. **Deduplicate and filter** (Go): hard rules, no model call.
4. **Extract** (Python): skills, years, seniority, visa, salary, language.
5. **Score** (Python): 0–100, reason, gaps, CV variant.
6. **Tailor** (Python): CV + cover letter (< 250 words) from the evidence bank, one-page PDFs.
7. **Track**: statuses, history, follow-ups, notes, contacts.

## Scoring

Filters, not hard rules (changed 2026-11-01, see roadmap M2-03): every job is classified
(role family, seniority, location and remote scope, explicit experience minimum, open or
closed) and filter presets decide what is shown. The owner's profile is the default
preset: target engineering roles, junior to senior, remote from Morocco or an accepted
city, or relocation offered, open, explicit experience minimum under 6 years. Nothing is
deleted; hidden jobs and the filter that hid them stay viewable.

| Factor | Weight |
|---|---|
| Required skills covered by proven skills | 35 |
| Title match | 20 |
| Seniority and years | 15 |
| Location, remote policy, visa | 15 |
| Optional skills and domain | 10 |
| Freshness | 5 |

Output: score, fit level (very strong / strong / good / stretch), a reason of ≤ 2
sentences, gaps, role family. Yes/no judgments go through one `decide()` interface.

## Tracker statuses

new, shortlisted, drafted, applied, replied, interviewing, offer, rejected, closed, skipped.
Follow-up prompt 7 days after applying with no reply.

## Guardrails

- Tailoring may select, reorder and rephrase evidence; never add skills, numbers, titles or dates.
- A second pass flags unsupported sentences. Titles and dates copied exactly.
- The owner sends everything. No logged-in scraping; LinkedIn via manual add only.
- Personal data stays in the owner's database; only needed text goes to a model.

## Milestones (8 weeks, part-time)

| Week | Phase | Done when |
|---|---|---|
| 0 | Walking skeleton | Repo, CI, migrations, VPS, one Lever job end to end, deployed |
| 1 | Collector | 50 watch-list companies collected into Postgres |
| 2 | Rules + plain tracker | Dedup and hard rules work; daily table of new leads (usable) |
| 3 | Extraction + scoring | Facts, score, reason, CV variant per lead |
| 4 | Evidence bank + tailoring | Master CV imported; tailored draft + coverage report |
| 5 | Documents | Cover letter, claim check, PDF export |
| 6 | Full tracker | Statuses, history, board view, reminders, manual add |
| 7 | Digest + sources | Morning digest; remote and Moroccan boards |
| 8 | Feedback + write-up | Skip reasons feed scoring; public repo and benchmarks |

Cut line: board view, digest and extra sources can slip; collection, scoring, tailoring
and tracking cannot. Keep applying for jobs while building.

## Targets

20+ relevant leads/week; 70%+ of top leads worth applying to; < 10 min from lead to
documents; 10 applications/week; 0 unsupported claims; < $20/month model cost.

## Open questions

Which 50 companies first; which boards allow automated collection; email or Telegram
digest; which model; auto-draft for top leads or on request; open to others later.
