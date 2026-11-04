# M3-05: Skill map and search profile

- **Status:** Ready
- **Milestone:** M3 — Extraction and scoring
- **Linear:** [HAM-44](https://linear.app/hamid-corp/issue/HAM-44/m3-05-skill-map-and-search-profile)
- **Estimate:** ½ day

## Story

As the job seeker, I want my skills (proven / familiar / learning) and my ranked target
titles (EN + FR) kept in one reviewed file, so that scoring compares each lead to what I
can actually show.

## Context

PRD origin: "ATS keyword clusters → skill map (proven / familiar / learning)" and "ranked
title list (EN + FR) → search profile". Scoring (M3-06) reads both. These are personal
data: they stay on the owner's machine (PRD guardrail) unless he decides otherwise.

## Acceptance criteria

1. **Given** `data/profile/skill_map.yaml` and `data/profile/search_profile.yaml`
   **When** they are loaded
   **Then** they are validated like the watch list and presets: every invalid line is
   reported with its line number before anything runs; skills have a level
   (proven/familiar/learning) and optional aliases; titles are ranked, with EN and FR
   forms.
2. **Given** skill names from extraction ("Postgres", "PostgreSQL", "postgresql")
   **When** they are matched against the skill map
   **Then** aliases and case/accents resolve to one skill (table test with real
   extracted names).
3. **Given** the files are personal
   **When** the repository is shared later (M8)
   **Then** the real files are git-ignored and committed `*.example.yaml` files show the
   format; the worker reads the real ones from a path in config.
4. **Given** a change to either file (or to the scoring weights or bands, M3-06)
   **When** the worker next runs
   **Then** it picks up the new version without a restart, and the **coded score and
   breakdown are recomputed for every shown lead** (no model call, free); each score
   records the versions (file hashes, weights and bands version) it came from. The
   model-written reason and gaps are not regenerated here (M3-06 AC5).
5. **Given** the personal files are git-ignored
   **When** I run `make backup`
   **Then** the same backup also contains `data/profile/*` and the eval posting texts
   (M3-07) in a separate archive next to the database dump, same timestamp, same 600/700
   modes and the same keep-14 rotation; `make restore` restores the database and prints
   how to put the personal files back (they are not overwritten silently). Tested in the
   smoke test with example files.

## Out of scope

- The evidence bank and Master CV (M4). Editing these files from the page.

## Security notes

Personal data: never sent to a model in full; scoring sends only what a call needs (M3-06).
Git-ignored real files; examples contain no personal data.
