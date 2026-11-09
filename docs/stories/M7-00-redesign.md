# M7-00: Redesign: split view, pill navigation, pastel cards

- **Status:** Done
- **Milestone:** M7 — Today view and more sources
- **Linear:** [HAM-68](https://linear.app/hamid-corp/issue/HAM-68/m7-00-redesign-split-view-pill-navigation-pastel-cards)
- **Estimate:** 2 days (gate, then build)

## Story

As the job seeker, I want a calmer, more visual tracker: navigation in one row of pills,
a list I can move through with the detail always beside it, and colours that tell me
fit and status at a glance, so that a daily look is quick and pleasant on the Mac and
on my phone.

## Context

Same process as M2-06: brief (`docs/design/brief.md`, M7-00 section), then a mockup gate
with two directions on real data, then the build, then a review loop with the vendored
skills (`frontend-design`, `web-design-guidelines`, `fixing-accessibility`). The owner's
references are screenshots of a friend's site. They are described in the brief and never
committed: they show real people.

## Acceptance criteria

1. **Given** the gate
   **When** the two directions are shown (Leads split view and Today, light and dark, 375
   and 1280 px, side by side, on real data)
   **Then** the owner picks one before any build.
2. **Given** the picked direction
   **When** the tracker loads
   **Then** it has:
   - pill navigation: Today · Leads · Board · Follow-ups · Drafts;
   - dark-first soft near-black surfaces with cards one shade lighter, radius 20–24 px;
     the light theme stays;
   - a pastel semantic palette: mint for strong fit or positive, sand for good or
     waiting, salmon for stretch, no or rejected, powder blue for info or applied. AA
     contrast is checked for every text and tag pair in both themes;
   - a self-hosted open-licence variable font (licence committed; CSP unchanged), bigger
     titles, tabular numbers.
3. **Given** the leads list
   **When** I click a row, or move to it with j/k or the arrow keys
   **Then** the row is selected and the detail panel on the right follows. The panel has
   tabs: Overview (fit, reason, gaps, breakdown, status), CV & letter, Notes, Contacts,
   History. The title is a real link to the posting (new tab,
   `rel="noopener noreferrer"`); clicking it doesn't select the row.
4. **Given** a phone (375 px)
   **When** I select a lead
   **Then** the detail opens as a full-screen sheet with a Back button. Back and Esc
   return to the list, at the same place.
5. **Given** the Today page (the M7-01 content in the new design)
   **When** it loads
   **Then** it shows coloured stat tiles, then new leads since the last visit, follow-ups
   due and closed postings, as rounded cards with a coloured tag and "time ago".
6. **Given** every page
   **When** Playwright runs at 375 and 1280 px, light and dark
   **Then** axe finds 0 violations and nothing scrolls sideways. The existing tests keep
   passing, adapted to the new structure.

## Results (2026-11-09)

- **Owner's pick:** direction A "Soft cards" with B's single rounded lead list and thin
  dividers (density for ~500 leads). Rows keep A's filled pastel fit tags; Today is fully A.
- **Font:** Geist variable (latin and latin-ext), vendored in `web/public/fonts/` with
  `OFL.txt` (from the Geist repository) and `SOURCE.md` (hashes); preloaded. CSP
  unchanged: `default-src 'self'` covers it.
- **Pages:** `?page=today|leads|board|followups|drafts` (Today by default). Older links
  (`?view=board`, plain filter links) still land on the right page. The selected lead
  and tab are in the URL (`lead=`, `tab=`); j/k replaces the history entry.
- **Leads:** the whole row selects and the title is a real link. On wide screens the
  first lead shows until you pick. On the phone the detail is a full-screen sheet:
  Back, Esc and the system Back close it, and focus returns to the row.
- **Detail:** tabs follow the ARIA pattern (arrows, Home and End).
  - Overview: the fit tile, the status tile with Skip, the reason, gaps, breakdown,
    facts and postings.
  - "CV & letter": the old draft dialog, now inline.
  - Notes, Contacts, History.
- **New pages:** Today (tiles, new leads, follow-ups due, closed), Follow-ups (its own
  page), Drafts (`GET /api/drafts`, each lead's newest draft; new golden files). The
  Board is restyled.
- **Review loop (`web-design-guidelines`):** fixed scroll-margin under the sticky bar,
  `content-visibility` on rows, balanced headings, hover states, `overscroll-behavior`
  and safe areas on the phone sheet, contact inputs (autocomplete, spellcheck), font
  preload, `translate="no"`, links (not buttons) for in-app navigation, and the
  `theme-color` meta. Kept sentence case (the project's style) over Title Case.
- **Tests:** Playwright rewritten for the new structure: 68 tests, 375 and 1280 px, light
  and dark, axe 0, no horizontal scroll. The contrast test now covers text on every
  surface, near-black on every pastel, and the accent at 3:1. Fixing it surfaced that the
  minifier shortens `#ffffff`.
- **Screens:** `docs/design/screens/m7-00/`, fixture data only
  (`docs/design/m7/shoot.mjs`). The gate mockups with real data stay in git-ignored
  `docs/design/m7/private/`.

## Out of scope

- New features beyond M7-01 (Drafts and Follow-ups pages list what exists already).
- The friend's brand: logo, copy, exact layout.

## Security notes

- The font is self-hosted: no new origin, CSP unchanged.
- Posting text is still rendered as text only, and links are http(s) only.
- Reference screenshots are not committed: they show real people.
