# 0013. Build the tracker page with Vite, React and TypeScript, embedded in the API binary

- **Status:** Accepted
- **Date:** 2026-11-02
- **Story:** docs/stories/M2-06-plain-tracker-table.md

## Context

M2-06 adds the first TypeScript: one page, opened every morning, that reads only the
M2-05 API. It must work on a phone, keep all filter state in the URL, and render
untrusted posting text safely. npm is a new supply chain for the repo, so every
dependency is a cost. The tool should stay one process on one VPS (ADR-0002: no auth,
Tailscale only).

## Decision

**Stack:** Vite + React + TypeScript in `web/`, no component library, no router, no
state or data-fetching library. Plain `fetch` against the API, filter state read from and
written to `URLSearchParams` (`history.pushState`), plain CSS (a table that becomes cards
under 640 px).

**Dependencies** (exact versions in the committed `package-lock.json`):

| Runtime | Dev |
|---|---|
| `react`, `react-dom` | `vite`, `@vitejs/plugin-react`, `typescript`, `@types/react`, `@types/react-dom`, `vitest`, `jsdom`, `eslint`, `typescript-eslint`, `@playwright/test`, `@types/node` (types for the test and build config) |

plus `@axe-core/playwright` (dev) for automated accessibility checks on every page in the
Playwright tests. No testing-library (Vitest + `react-dom` + `jsdom` are enough), no
`eslint-plugin-react`: the ban on `dangerouslySetInnerHTML` is ESLint's built-in
`no-restricted-syntax` on that JSX attribute.

**Fonts:** the system font stack (and the system monospace stack for numbers where
needed). At most one self-hosted variable font may be added later, served from the
binary; never a font CDN, so the CSP stays same-origin.

**Supply chain:** Node pinned to the 26 LTS line (LTS since 2026-10-28): `web/.nvmrc`
says `26` (`.npmrc` sets `engine-strict`, so `npm ci` refuses another Node), `engines` says `>=26 <27`, and CI's `setup-node` installs the exact version
(`26.13.0`, bumped by hand like the Go tools); `npm ci` from the committed lockfile only; `npm audit
--audit-level=high` in `make check`; Dependabot `npm` updates for `/web`, grouped and
weekly; actions pinned by SHA as today.

**Serving:** `vite build` writes to `web/dist/app/`. A Go package `web` embeds
`//go:embed all:dist` (`web/dist/.gitkeep` is committed, `dist/app/` is gitignored), so
the Go build never depends on Node having run; when the page isn't built, `/` answers
"run make web-build". The API serves the page at `/` (SPA fallback to `index.html` for
non-`/api` paths) with:

```
Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self';
  img-src 'self' data:; connect-src 'self'; base-uri 'none'; form-action 'none';
  frame-ancestors 'none'
```

plus `X-Content-Type-Options: nosniff` and `Referrer-Policy: no-referrer`. Development uses
`make web-dev` (Vite dev server proxying `/api` to the API on 127.0.0.1:8080).

**Untrusted text:** React text nodes only; links rendered only when the URL parses with
protocol `http:` or `https:`, always `target="_blank" rel="noopener noreferrer"`.

**Checks in `make check`:** `web-check` = `npm ci`, `npm audit --audit-level=high`,
`tsc --noEmit`, ESLint, Vitest, `vite build`, and Playwright (Chromium only) at 375 px and
1280 px against the built page with the API mocked by route interception, so CI never
needs a database for the UI tests. Every Playwright page also runs an axe check and
fails on any violation. CI caches Playwright's Chromium (`~/.cache/ms-playwright`) keyed
on the `@playwright/test` version in `package-lock.json`, so the browser is downloaded
only when Playwright is bumped. The Go tests for the embedded handler and CSP run
against the built files.

## Alternatives considered

- **Server-rendered Go templates + a little JS:** no npm at all, but filters, URL state
  and expandable groups become hand-written DOM code; the story names TypeScript.
- **Preact / htmx / Svelte:** smaller or different, but React is the most familiar to the
  owner and the size difference doesn't matter on a tracker used by one person.
- **A component library (MUI, shadcn):** dozens of transitive dependencies for one page.
- **Serving `web/dist` from disk or a separate static server / Caddy:** two artefacts to
  deploy and keep in step; embedding keeps the tool one binary.
- **Next.js / Remix:** a server runtime we don't need; the API is the server.

## Consequences

- **Good:** one binary to deploy; the page can only call its own origin; every filter
  view is a bookmarkable URL; a small, auditable dependency list.
- **Bad:** a Node toolchain in CI and for anyone building the page; Playwright's Chromium
  (~150 MB) is downloaded whenever Playwright is bumped; React is heavier than the page
  strictly needs.
- **Revisit when:** the page grows into the M6 tracker (routing, mutations) and the
  hand-rolled URL state or fetch code becomes the bulk of the code.

## Owner decisions (2026-11-02)

- Node 26 (LTS since October 2026), exact version in CI.
- Chromium cached in CI, keyed on the Playwright version.
- `@axe-core/playwright` added for accessibility checks.
- System fonts, or at most one self-hosted variable font; no font CDN.
