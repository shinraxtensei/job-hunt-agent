# Design brief: the tracker page (M2-06)

## Who uses it

One person: me, the job seeker. Software engineer, ~3.5 years, based in Casablanca, open
to remote and some relocation. I open the page **every morning**, mostly at a laptop
(1280–1600 px, light room, coffee), sometimes on my phone (375 px) to check something on
the go. I'm the only user until milestone S; friends in sales or marketing may use their
own presets later, so the page must not assume engineering vocabulary everywhere.

## The material

- ~4,500 duplicate groups stored; **~500 shown** by my default preset on a given day,
  ~4,000 hidden by it. Most mornings only a few dozen are new.
- Titles are 40 characters on average, up to ~105; many are French ("Développeur/se
  Fullstack - CDI Paris - Theodo GovTech"). Locations: 2.3 per lead on average, up to
  34 ("Romania; Brazil; Peru; …"). ~600 groups fold several postings.
- 50 companies, four tracks (remote, relocation, morocco, consulting).
- Every string comes from job boards: untrusted, sometimes long, sometimes empty, never
  formatted for us.

## Main tasks, in order of frequency

1. **Scan:** what's new since yesterday, newest first; recognise company, role, place and
   seniority in one glance per row.
2. **Open a posting:** one keystroke or click to the board's page, in a new tab.
3. **Filter:** narrow or widen by role family, seniority, workplace, remote scope,
   country, track, company, experience, new; see how many leads each value would add.
4. **Check what my filters hide**, and why, then widen a filter if something good is in
   there. (Per-lead "hide/skip" arrives with the full tracker in M6; in M2 "hide" means
   the filters and the hidden view.)
5. **Reset to my defaults** after exploring.
6. Check a merge: expand "N postings" to see the folded duplicates.

## Density and tone

- **Dense like an inbox, calm like a document.** At 1280 px, at least 15 rows visible
  without scrolling, one line per lead where it fits. Numbers and dates aligned
  (tabular figures).
- Quiet by default: neutral surfaces, one accent used only for what needs action or
  attention (new leads, focus, the selected filter). Hierarchy from type weight and
  spacing, not boxes and colour.
- Fast: no spinners longer than they need to be, no layout jump when data arrives,
  motion only to show what changed (≤200 ms, transform/opacity, off under
  reduced-motion).
- Keyboard-first on the laptop (j/k, o/Enter, /, Esc, ?), thumb-friendly on the phone.
- Honest copy: plain sentence case, counts that add up ("512 shown · 3,974 hidden by
  these filters"), errors that say what to do.

## Anti-goals

- Not a dashboard: no charts, KPI tiles, or "insights".
- Not a marketing page: no hero, no illustrations, no brand flourishes.
- No card grid on desktop: tabular data goes in a real table.
- No gradients, glassmorphism, glow shadows, emoji icons, default purple, or the
  Inter-on-white SaaS template look.
- No component-library look, no icon soup, no hover-only controls.
- Nothing that requires a network font, an external script or style (the CSP is
  same-origin only).
- No dark patterns, no gamification, no "streaks".

## Constraints

- Plain CSS with design tokens (custom properties), light and dark themes following
  `prefers-color-scheme` with a manual toggle; WCAG AA contrast in both.
- System font stack, or at most one self-hosted variable font.
- Works at 375 px without horizontal page scroll; filters in a sheet on the phone.
- Untrusted text rendered as text only; links only http(s).

## Reference (owner, 2026-11-02)

**Linear** is the primary and only visual reference. The page should *feel* like Linear,
not look like a clone.

Take from it:

- compact list rows;
- quiet chrome;
- one accent colour;
- a strong dark mode;
- keyboard-first: j/k, o/Enter, /, Esc, ?;
- a command-style search;
- subtle hover and focus states;
- filter chips that show what's applied.

Don't copy: its logo, name, icons, exact layout or colours.

Not Jira: no heavy toolbars, no field-dense forms.
