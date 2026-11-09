# M7-02: Moroccan employers: Personio feed and French drafts

- **Status:** In progress
- **Milestone:** M7 — Today view and more sources
- **Linear:** [HAM-66](https://linear.app/hamid-corp/issue/HAM-66/m7-02-moroccan-employers-personio-feed-and-french-drafts)
- **Estimate:** 1 day (split if SmartRecruiters is accepted: M7-02b)

## Story

As the job seeker in Morocco, I want Moroccan employers' postings collected
automatically where their terms allow it, and drafts in French for French postings
written only from French evidence lines I checked, so that I can apply locally as easily
as remotely.

## Context

Step 0 (`docs/research/m7-sources.md`) found no Moroccan job board whose terms allow
automated reads: ReKrute and Welcome to the Jungle forbid it, and Emploi.ma blocks
automated fetches. Moroccan employers are reached through their own career feeds
instead, like Lever, Greenhouse and Ashby today:
- **Personio's public XML feed** (Inyad: 22 postings in Casablanca);
- **SmartRecruiters' Posting API** (Devoteam, Alten, Delivery Hero) only if the owner
  accepts it (OI-18). Its API host's `robots.txt` disallows all but LinkedInBot.

Letters are English-only (M5 OI-7). French postings need French evidence, written and
checked by the owner, never machine-translated (owner, 2026-11-08).

## Owner input

- **OI-17:** build the Personio adapter (default: yes).
- **OI-18:** SmartRecruiters (default: no; those companies stay manual-add).
- **OI-21: French evidence lines** in `data/profile/evidence.yaml`. Optional per line: a
  `text_fr` field with the same facts in French, written and checked by you (`checked:
  true` covers both texts). Example:

  ```yaml
  - id: example-billing
    text: "Rebuilt the billing service's invoice job, cutting its run time from 40 to 6 minutes."
    text_fr: "Refonte du traitement de facturation : durée réduite de 40 à 6 minutes."
    checked: true
  ```

  Also `letter_fr` lines (the letter-only lines in French), and `person.headline_fr`.

## Acceptance criteria

1. **Given** a watch-list company with `board_type: personio`
   **When** `make collect` runs
   **Then** the Personio adapter fetches `https://{slug}.jobs.personio.de/xml` and
   returns its postings as `job.Raw`. Same contract and politeness as the other
   adapters; golden-file, empty, malformed, HTTP-error, cancellation and fuzz tests. Inyad
   is added to `data/companies.yaml` (track: morocco).
2. **Given** a posting whose extracted language is French
   **When** a draft is requested
   **Then** if every selected line has a checked `text_fr`, the CV and letter are written
   in French from the French texts only:
   - French prompts (`prompts/tailor/v2.md`, `prompts/letter/v2.md`);
   - French greeting and sign-off by code;
   - the validator and claim checker see the French texts as evidence;
   - numbers must match the French text exactly.
   Otherwise the draft is in English and says why ("no French evidence for: …").
3. **Given** French evidence
   **When** it is loaded
   **Then** a `text_fr` without `checked: true` is refused like any line. The validator
   also checks the numbers of `text` and `text_fr` match each other (a translation slip
   would let a number change).
4. **Given** the eval set
   **When** `make evals-tailor` and `make evals-checker` run
   **Then** French drafts for the French eval postings are reported separately, and the
   guardrail cases stay at 100% in both languages.

## Results

**Slice 1: Personio (2026-11-09).**
- **Adapter:** `internal/source/personio` reads `https://{slug}.jobs.personio.de/xml`
  with Go's XML decoder (no external entities), the shared fetch client (size cap,
  retries, Retry-After) and one limiter for all of Personio's subdomains. The slug is
  checked before any request (it becomes a host name). Each position is stored as a JSON
  object of its fields.
- **Tests:** golden file (Inyad, 2026-11-09, trimmed to 3 positions), empty feed,
  malformed positions skipped and counted, not-a-feed errors, bad slugs refused, HTTP
  errors naming the company, and a fuzz test (seeded with an external-entity document).
- **Normaliser:** `job.Normalise("personio", …)`: title, URL, offices, department,
  employment type, created date; an office named Remote means remote. Description text
  goes to the classifier.
- **Watch list:** `personio` is a board type (migration, watch-list loader, collector
  registry); Inyad is on the watch list (track: morocco).
- **Live:** 25 Inyad postings collected; the default preset shows the 2 engineering roles
  in Casablanca and hides the rest for role reasons.

## Out of scope

- Machine translation of evidence: never. Moroccan job boards (their terms forbid it):
  their postings enter by manual add.

## Security notes

- **Personio:** public XML, no login; `robots.txt` allows all. Personio's own help pages
  couldn't be read (403); if they forbid third-party reads, the adapter is removed.
- **SmartRecruiters:** not built unless OI-18 says so.
- **Untrusted input:** feed XML is parsed with Go's `encoding/xml` (no external
  entities), size-capped, and fuzzed.
- **French evidence** is personal, like the rest of the bank.
