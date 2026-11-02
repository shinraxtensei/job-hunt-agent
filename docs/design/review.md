# Review loop: tracker page, direction A (M2-06b)

Run on 2026-11-02 against `web/src` after the build, with the vendored skills
(`.claude/skills/`, ADR-0014). impeccable was rejected in review (ADR-0014, PR #25), so
its audit/critique pass is replaced by the frontend-design self-check below.

## 1. frontend-design self-check (critique)

| Finding | Response |
|---|---|
| Accent used on checked checkboxes in the filter sheet (third use of blue) | **Fixed:** `accent-color: var(--text)` (`web/src/styles/app.css:577`). Blue is only on the focused row, the new-dot and focus rings. |
| Template tells: ALL-CAPS labels, eyebrows, numbered markers, gradients, glow, cards on desktop, emoji icons, default purple, Inter | None present. Section labels are sentence case; the list is a `<table>`; icons are inline SVG strokes; system font stack. |
| Meta strings joined with "·" (location · workplace) | **Kept:** two facts of one cell, scanned together; one separator style throughout. |
| "Spend boldness in one place" | The focused row (background + 2 px accent bar) is the one strong element; everything else is grey on neutral. |
| Copy | Sentence case, plain verbs ("Reset to my defaults", "Try again", "Clear search"); errors say what to do; counts add up ("512 shown · 3,974 hidden by these filters"). |
| Motion | Only the detail row reveal and the chevron rotation, 150 ms, transform/opacity, 0 ms under `prefers-reduced-motion` (`web/src/styles/tokens.css`). |

## 2. web-design-guidelines (`file:line`)

Fixed:

```text
web/src/styles/app.css:201 - search input outline:none replaced only by a subtle border → accent outline on :focus-within
web/src/styles/app.css:377 - sticky top bar could cover the row focused with j/k → scroll-margin-top on rows
web/src/styles/app.css (.row:focus-visible) - keyboard-focused row had no accent bar on desktop → added
web/src/FilterSheet.tsx:65 - select without name → name="preset"
web/src/FilterSheet.tsx:95 - select without name → name="closed"
web/src/LeadTable.tsx:164 - brand names auto-translated → translate="no" on company
web/src/App.tsx:450 - straight apostrophe in copy → "Couldn’t"
web/src/LeadTable.tsx:234 - straight apostrophe in copy → "Couldn’t"
web/index.html:6 - no <meta name="theme-color"> → one per colour scheme
web/src/App.tsx:44 - search not in the URL → q kept with replaceState (not one history entry per keystroke)
```

Justified, not changed:

```text
web/src/LeadTable.tsx:154 - <tr onClick> (click handler on a non-button) → it only moves the roving row focus for the mouse; the keyboard equivalent is j/k, and the row's actions are a real <a> and <button>
web/src/App.tsx:15 - "large lists (>50): virtualize" → ~500 rows of text render in one pass; table virtualization would break native table semantics, find-in-page and screen-reader table navigation. Revisit if the shown list passes ~2,000.
rules.md "Title Case for headings/buttons" → the project follows sentence case (frontend-design; noted in web-design-guidelines/SOURCE.md)
rules.md "deep-link all stateful UI" → filters, view and search are in the URL; expanded rows and the focused row are not (transient)
```

## 3. fixing-accessibility

| Rule | Finding | Response |
|---|---|---|
| accessible names | icon-only buttons (rail, theme, close, chip remove, row details) | all have `aria-label`; icons are `aria-hidden` |
| semantics | no `<h1>` when the sidebar is a rail (axe `page-has-heading-one` at 375 px) | **Fixed:** the h1 is visually hidden in rail mode instead of `display:none` |
| semantics | `aria-label` on a plain `div` (axe `aria-prohibited-attr` on `.chips`) | **Fixed:** `role="group"` |
| media and motion | new-dot meaning was colour + title only | **Fixed:** visually hidden "New" text inside the dot (`web/src/LeadTable.tsx:160`) |
| focus and dialogs | filter sheet and shortcuts | native `<dialog>` with `showModal()`: focus trapped, Esc closes, focus returns to the trigger |
| announcements | loading, counts, member loading | `role="status"` (+ `aria-busy` while loading); errors use `role="alert"`; expand buttons use `aria-expanded` + `aria-controls` |
| keyboard access | every control reachable by Tab; no `tabindex > 0`; skip link to the leads | yes |

## 4. Automated checks (in `make check` and CI)

- **axe**: `@axe-core/playwright` on every Playwright page (default list, filter sheet,
  hidden view, expanded group, shortcuts dialog, error and "everything hidden" states)
  at 375 and 1280 px in light and dark: **0 violations**.
- **Contrast**: `e2e/tracker.spec.ts` "token colours meet WCAG AA in light and dark"
  computes the ratios of `--text`, `--muted` and `--faint` on `--bg`, `--panel`,
  `--hover` and `--row-focus` (≥ 4.5:1) and of `--accent` on `--bg` and `--row-focus`
  (≥ 3:1, non-text) in both themes. axe's `color-contrast` rule checks the rendered page.
