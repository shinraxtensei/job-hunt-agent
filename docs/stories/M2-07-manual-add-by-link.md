# M2-07: Manual add by link

- **Status:** Ready
- **Milestone:** M2 — Rules and a plain tracker
- **Linear:** [HAM-36](https://linear.app/hamid-corp/issue/HAM-36/m2-07-manual-add-by-link)
- **Estimate:** 1 day

## Story

As the job seeker, I want to add a job I found elsewhere by pasting its link, so that
every role I care about ends up in the same tracker, including ones from LinkedIn or a
company I don't watch.

## Context

PRD: "Manual add: paste any job link and it enters the pipeline at the normalise stage",
and "LinkedIn postings enter only through manual add". Interface in M2 is a CLI
(`collector add <url>`), because the tracker is read-only until M6.

## Acceptance criteria

1. **Given** a link to a posting on Lever, Greenhouse or Ashby
   (`jobs.lever.co/<site>/<id>`, `boards.greenhouse.io/<board>/jobs/<id>`,
   `job-boards.greenhouse.io/...`, `jobs.ashbyhq.com/<board>/<id>`)
   **When** I run `collector add <url>`
   **Then** the posting is fetched through that board's adapter and public API (with the
   shared limiter), stored and normalised like a collected one, and its company is
   created if new (`active = false`, so it isn't collected daily unless I add it to the
   watch list).
2. **Given** any other link (LinkedIn, a company careers page, a Moroccan board)
   **When** I run `collector add <url> --title "..." --company "..." [--location "..."]`
   **Then** a manual posting is stored with what I typed. **The link is never fetched.**
   Without `--title` and `--company` the command refuses and says what is missing.
3. **Given** the URL parser
   **When** it is table-tested with valid links, look-alike hosts
   (`jobs.lever.co.evil.com`, `evil.com/jobs.lever.co/...`), userinfo tricks
   (`https://jobs.lever.co@evil.com/...`), non-http schemes and IP addresses
   **Then** only exact known hosts are treated as boards; everything else is manual.
4. **Given** a manually added board posting
   **When** two days pass
   **Then** a recheck (part of `collector run`) fetches it individually and marks it closed
   if it's gone (404), reopened if it's back. Manual non-board postings are never
   rechecked automatically.
5. **Given** the same link added twice
   **When** the second add runs
   **Then** no duplicate is created; the command says it already exists.

## Out of scope

- Adding from the web page (M6). Fetching or parsing pages from unsupported sites.

## Security notes

This is the one place a user-supplied URL could make the collector fetch something: the
SSRF risk. The rule is absolute: only URLs whose exact host is a supported board are
fetched, and only through that adapter's fixed API endpoint, never the pasted URL itself.
AC3's hostile-URL table test is required.
