# M5-03: PDF export

- **Status:** Ready
- **Milestone:** M5 — Documents
- **Linear:** [HAM-57](https://linear.app/hamid-corp/issue/HAM-57/m5-03-pdf-export)
- **Estimate:** 1 day

## Story

As the job seeker, I want an approved draft as two one-page PDFs (CV and letter) that
ATS software can read, so that I can send them as they are.

## Context

PRD: "one-page PDFs". Only an approved draft (M4-05) is exported; the PDF holds exactly
the stored, validated and checked text. A PDF library is a new dependency: ADR-0027.

## Owner input

**OI-8**: the PDF library (default `fpdf2`, pure Python). **OI-4** (layout) still
applies; defaults until set.

## Acceptance criteria

1. **Given** a draft
   **When** the owner approves it
   **Then** the worker renders the CV and the letter as A4 PDFs from the stored content
   only: no model call, no text the draft doesn't hold.
2. **Given** a rendered CV
   **When** it is longer than one page
   **Then** the lowest-ranked bullets are dropped, one at a time, until it fits. A
   sentence is never cut. If it still doesn't fit with one bullet per section, export
   fails with a message and nothing is stored.
3. **Given** a PDF
   **When** its text is extracted (pypdf, test-only dependency)
   **Then** the text matches the draft, in reading order, with real text (no images of
   text), and accented characters survive (ATS check).
4. **Given** draft text that contains markup or control characters
   **When** it is rendered
   **Then** it appears as literal text; nothing is interpreted (no links, no scripts,
   no embedded files).
5. **Given** an exported draft
   **When** I open it on the review page
   **Then** I can download each PDF (`GET /api/documents/{id}/pdf`, attachment, file
   name `<Name>-CV-<Company>.pdf`).

## Decisions (owner, 2026-11-08)

- OI-8: `fpdf2`, `pypdf` test-only (ADR-0027).
- fpdf2's built-in fonts are Latin-1 only. A vendored open-licence TTF is embedded, with
  its licence file committed, so accents and non-Latin names render (é, ç, ü, "Côte
  d'Ivoire"). A test exports a draft with accented text and checks the text layer with
  pypdf.

## Out of scope

- Several templates or themes (one plain, ATS-safe layout). DOCX.

## Security notes

- New dependency (ADR-0027): pure Python, no native code, fed only our own stored text
  and a bundled font (OFL).
- The download is a GET of the owner's own data; same Host allowlist as every route.

## Notes for the implementer

- Font: a vendored OFL TTF (licence committed), embedded. Page count checked by the renderer.
