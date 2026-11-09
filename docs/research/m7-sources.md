# M7 sources: can we collect from them? (source-adapter step 0, 2026-11-09)

Checked: official API or feed, terms of use, robots.txt, login, anti-bot. Only sources
whose terms allow automated reads are built (owner, 2026-11-09). Fetched with a
descriptive User-Agent; nothing logged in. Quotes are short and dated; terms change.
This is not legal advice.

## Moroccan job boards

| Source | API / feed | Terms | robots.txt | Verdict |
|---|---|---|---|---|
| ReKrute | none found; `/rss` 404 | `mentions-legales.html` forbids extracting its databases ("d'extraire … les bases de données"), deep links, any use without written consent | no `*` group; named search and AI bots allowed except on `/offres.html` | **No**: terms forbid |
| Welcome to the Jungle | none public (search uses private Algolia keys) | CGU (27 Apr 2026), clause 10 bans scraping robots; clause 19.2 limits job content to personal use | `Disallow: /*?` | **No**: terms forbid |
| Emploi.ma | `/rss` 404 | terms page answers automated fetches with 403 | allows `*`, blocks AI crawlers | **No**: blocks automated reads (step 0: stop) |
| Bayt | none | not needed: robots blocks the listings | `Disallow: /en/jobs/?`, `/fr/jobs/*-jobs/` | **No** |
| Indeed, LinkedIn | none for us | forbid automated access | restrictive | **No**: manual add only |
| Optioncarriere.ma | `search/rss.html` | not read | `Disallow: /job/`, `/search/rss.html` | **No** |
| Dreamjob.ma | WordPress RSS `/feed/` | no terms page found (privacy page 403) | allows `*` except admin | **Skip**: mostly public-sector "concours", not tech; its content also carried text addressed to AI readers |
| ANAPEC | none | not read | redirects | **Skip**: public employment agency, not tech-focused |

**No Moroccan job board can be built.** Moroccan employers are reached through their own
applicant-tracking systems' public career feeds, like Lever, Greenhouse and Ashby today.

## Company career feeds (ATS) covering the backlog

| ATS feed | Status | Backlog companies found | Verdict |
|---|---|---|---|
| Personio XML `https://{company}.jobs.personio.de/xml` | public, no login; `robots.txt`: `Allow: /`; Personio's help pages (403) not read, developer docs don't describe it | **Inyad**: 25 postings, 22 in Casablanca (the "HPS" on Personio is an unrelated German firm) | **Build** (owner question OI-17) |
| SmartRecruiters Posting API `api.smartrecruiters.com/v1/companies/{id}/postings` | official API, works without a key; docs: for customers' "fully customizable career sites"; 10 req/s | **Devoteam** 960 (60 in Morocco), **Delivery Hero** 903 (6), Endava 163 (0), Picnic 2; not on the backlog: **Alten** 1,251 (165 in Morocco) | **Ask**: `robots.txt` on the API host is `Disallow: /` for every agent but LinkedInBot (OI-18) |
| Recruitee, Breezy | public JSON | Glovo and Capgemini accounts list 0 jobs | No value now |
| Workable | widget endpoint lists 0 jobs (M1-01) | Chari, Glovo, Zenika, Dial, Vinted… | stays backlog (M1-06) |

Not found on any public feed: Yakeey, YoLa Fresh, YouCan, Freterium, DabaDoc, HPS,
Capgemini, Globant, SQLI, Xebia, Neoxia, Nimbleways. They stay manual-add (`collector
add <link>`).

## Remote boards

| Board | API / feed | Terms | Verdict |
|---|---|---|---|
| Remotive | public API `remotive.com/api/remote-jobs` | notice in the API: link back and name Remotive; don't resubmit to third-party sites; at most ~4 fetches a day; 24 h delay | **Build**: personal tracker, link and source shown, 1 fetch a day |
| RemoteOK | public API `remoteok.com/api` | API notice and `/legal` (26 Oct 2026): link back and name Remote OK; no logo | **Build**: link and source shown |
| We Work Remotely | RSS | terms page answers 403: unread | **Skip until terms are read** (OI-19) |
| Himalayas | public JSON API | no terms found | **Skip until terms are read** (OI-19) |
| Working Nomads | public JSON | no terms found | **Skip until terms are read** (OI-19) |
