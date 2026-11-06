# M4-05: Draft review page

- **Status:** Ready
- **Milestone:** M4 — Evidence bank and tailoring
- **Linear:** [HAM-53](https://linear.app/hamid-corp/issue/HAM-53/m4-05-draft-review-page)
- **Estimate:** 1 day

## Story

As the job seeker, I want to open a lead, ask for a CV draft, and review it next to its
evidence and coverage, so that I can trust every line before I use it.

## Context

This is the tracker page (ADR-0013, direction A, the same design rules and
vendored-skills review as M3-08). It shows M4-02's drafts and M4-03's coverage. The
owner approves; nothing is sent (guardrail rule 6). The PDF is M5.

## Owner input

**OI-4** can be decided here: the first draft shows the layout in use, and the owner
keeps it or edits `cv_layout.yaml`.

## Acceptance criteria

1. **Given** a scored lead
   **When** I open it
   **Then** I can ask for a CV draft. While it is being made the page says so (polling
   like the reason), and the request is the ADR-0022-style POST.
2. **Given** a draft
   **When** it is shown
   **Then** the page shows:
   - the header and exact facts as stored;
   - the summary, the bullets per role and project, and the skills, in the layout's
     order;
   - on each bullet, its evidence: the cited line ids, and the line text on focus or
     tap, with the differences visible (rephrased, not copied);
   - the draft's status (`ready for review` or `blocked`) and its versions.
3. **Given** a blocked draft
   **When** it is shown
   **Then** each violation is next to its sentence, with the rule in words ("This
   number isn't in the cited evidence") and the closest evidence line. A blocked draft
   can't be approved.
   - Warnings (scope inflation, M4-04) are shown the same way, marked as warnings
     ("'Led' isn't in the cited evidence"). They don't block approval.
4. **Given** the coverage report
   **When** it is shown
   **Then** it lists covered skills (with lines), listed-only skills, and gaps (with the
   reason: learning, missing, no evidence). A suspicious skill name from the posting
   is flagged as a possible injection.
5. **Given** a ready draft
   **When** I approve or discard it
   **Then** its status changes (`approved` or `discarded`), and nothing is sent or
   exported. Older versions stay viewable.
6. **Given** model output and evidence text
   **When** they are rendered
   **Then** they are text only (the M2-06 AC4 tests: an HTML or `javascript:` payload is
   inert). Playwright at 375 and 1280 px, light and dark: axe 0, no horizontal scroll.

## Out of scope

- Editing bullets by hand, PDF export, cover letter (M5). Applications and statuses (M6).

## Security notes

- Writes go through the ADR-0022 guards: loopback Host, cross-origin protection, JSON
  only.
- The page renders CV content and model output as text only.
