# M5-03: PDF export

- **Status:** Done
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

## Results (2026-11-08)

- **Correction to OI-8:** fpdf2 depends on Pillow (native code), so it isn't "pure Python, no
  native code". The owner kept it, on two guards (ADR-0027, Accepted): no image is ever
  parsed (ruff ban plus a test), and `make vuln` scans the Python lockfile with `uv audit`.
- Versions are pinned below releases under two weeks old: fpdf2 2.8.9, fontTools 4.67,
  pypdf 6.21.4 (test only; 6.20 had known issues the new scan caught). Pillow 13.0 comes with fpdf2.
- The worker exports each approved draft once, inside its loop. No model call and no new
  work kind. A draft that can't fit records `export: …` in `drafts.error` (shown on the
  page) and isn't retried.
- `clean()` turns control characters into spaces. fpdf2 alone drops them, gluing words
  together.
- Download: `GET /api/documents/{id}/pdf`, an attachment named
  `<Name>-CV|Cover-letter-<Company>.pdf`, with letters and digits kept in any script.
- Each check was broken on purpose and its test seen to fail (in the PR).

## Out of scope

- Several templates or themes (one plain, ATS-safe layout). DOCX.

## Security notes

- New dependency (ADR-0027): pure Python, no native code, fed only our own stored text
  and a bundled font (OFL).
- The download is a GET of the owner's own data; same Host allowlist as every route.

## Notes for the implementer

- Font: a vendored OFL TTF (licence committed), embedded. Page count checked by the renderer.
