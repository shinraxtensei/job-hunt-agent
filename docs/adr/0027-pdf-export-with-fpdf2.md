# 0027. PDF export with fpdf2 and a vendored Noto Sans font

- **Status:** Accepted (owner, 2026-11-08: keep fpdf2 with Pillow, with the two guards
  below)
- **Date:** 2026-11-08
- **Story:** docs/stories/M5-03-pdf-export.md

## Context

M5-03 turns an approved draft into two one-page A4 PDFs. They must be readable by ATS
software (real text, not images), keep accents and non-Latin names, and contain nothing
active. The worker (Python) makes them from the stored text; nothing else may be added.

## Decision

- **fpdf2 2.8.9** (pinned below 2.9, which is under two weeks old) draws the pages with
  `multi_cell`: plain text, no markdown, no links. Its dependencies:
  - **fontTools 4.67** (pinned below 4.68, for the same reason): pure Python, reads the
    font;
  - **Pillow 13.0**: **native code**, a hard dependency of fpdf2 for images. We never
    pass it an image, so its decoders aren't reached, but it is in the image.
  - **defusedxml** (pure Python, for SVG; unused).
- **pypdf 6.21.4** is a test-only dependency (6.20 had 14 known issues, all fixed in
  6.21.4; a security fix beats the two-week age rule): the tests read the text layer back (ATS
  check, accents) and look for active content.
- **Noto Sans** Regular and Bold, static TTFs under the SIL OFL 1.1, are vendored in
  `agents/src/jobhunt_agents/fonts/` with `OFL.txt` and `SOURCE.md` (URL, SHA-256).
  fpdf2's built-in fonts are Latin-1 only.
- The page holds text only. Control characters become spaces. The tests check there is
  no `/JavaScript`, `/URI`, `/Annots`, `/EmbeddedFile` or `/Launch`, and that fpdf2's
  `/OpenAction` is only the initial view.

### Guard 1: no image is ever parsed

Pillow is in the image but must stay unreachable. Two checks run in `make lint` and
`make test-py`:
- **ruff TID251** (`banned-api` in `agents/pyproject.toml`) refuses importing `PIL`,
  `fpdf.html`, `fpdf.svg`, `fpdf.image_parsing`;
- **`tests/test_no_images.py`** walks the agents' code and refuses calls to fpdf2's
  image entry points: `image()`, `write_html()` (it loads `<img>`),
  `set_page_background()`, `template()`, `svg()`, and the same imports.

### Guard 2: Python dependencies are scanned like Go's

`make vuln` (part of `make check`, so CI) runs `uv audit` over `agents/uv.lock` (OSV)
next to govulncheck. `agents/tools/audit_fixable.py` lists every finding and fails when
one has a fix available, or when the scan produced no report. Its first run caught the
pypdf pin above.

## Alternatives considered

- **ReportLab:** also depends on Pillow; larger.
- **WeasyPrint (HTML/CSS):** needs Pango, Cairo and HarfBuzz in the image: more native
  code than Pillow.
- **Typst (a binary):** best typography, but a separate native binary to pin and update,
  and a markup language to escape.
- **Headless Chromium:** far heavier.

## Consequences

- **Good:** the smallest working option; real text; the font covers Latin, Greek and
  Cyrillic.
- **Bad:** Pillow's native code is in the worker image (Dependabot and `uv` pins track
  it). The font adds ~1.2 MB to the repo.
- **Revisit when:** the CV needs richer layout (columns, a photo), or a Pillow CVE can't
  be patched quickly.
