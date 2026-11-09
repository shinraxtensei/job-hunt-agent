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

---

# Design brief: the redesign (M7-00, 2026-11-09)

## Why

v1 works, but it reads as a dense admin table. The owner uses it daily, often on the
phone. The redesign keeps the keyboard-first speed of M2-06 and adds calm, colour and a
detail view that's always at hand.

## Reference (owner, 2026-11-09)

The references are screenshots of a friend's site: a student-stats dashboard with a
landing page, a corrections list, and a stats page. They are **not committed**: they show
real people's names and photos. What we take from them is the **style, not the brand**.
No logo, copy or layout is reproduced.

Take:
- **Surfaces:** dark-first, soft near-black backgrounds; cards one shade lighter, no
  hard borders; large radius (~20–24 px); generous spacing.
- **Colour:** pastel semantic colours on dark: mint, sand, salmon, powder blue, lilac.
  They fill stat tiles and tags; text on them is near-black.
- **Navigation:** pill buttons in one row at the top; the active pill is filled and the
  others are quiet.
- **Lists:** each item is a rounded card, with the main line bold, a quiet secondary
  line, a coloured tag on the right and "time ago" under it.
- **Stats:** coloured tiles with a small label, a very large number and a one-line
  comparison under it.
- **Type:** a bold modern sans; big page titles; tabular numbers.

Don't take: the horse logo, the wordmark, the green radial glow, the exact
three-column layout, any avatar or photo treatment.

## Semantic palette (both themes; AA checked)

| Meaning | Colour | Used for |
|---|---|---|
| strong / positive | mint | very strong and strong fit; offer; covered skills |
| good / waiting | sand | good fit; drafted, waiting; follow-up due |
| stretch / no | salmon | stretch fit; rejected, skipped, closed; blocked draft |
| info / applied | powder blue | applied, replied, interviewing; info tiles |
| neutral | lilac | shortlisted; tags without a meaning |

## Structure

- **Pills:** Today · Leads · Board · Follow-ups · Drafts. Search, filters and theme go on
  the right.
- **Leads:** split view, the list left and the detail right. Tabs: Overview · CV & letter
  · Notes · Contacts · History. The whole row is selectable; the title is a real link.
  j/k moves and the panel follows. On the phone the detail is a full-screen sheet with
  Back.
- **Today:** stat tiles (new since last visit, follow-ups due, applied this week, drafts
  ready), then new leads, follow-ups due and closed postings.

## Constraints (unchanged from M2-06)

- Plain CSS with tokens; light and dark; WCAG AA.
- One self-hosted variable font, its licence committed; CSP unchanged.
- 375 px without horizontal scroll.
- Untrusted text as text only.
- Mockups and their screenshots use real data, so they stay out of git
  (`docs/design/m7/private/`). Only the build script is committed.
