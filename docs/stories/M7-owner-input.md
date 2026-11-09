# M7 owner input

Each item has a default. Reply "defaults" to take them all.

**Answered 2026-11-09: defaults for OI-17 to OI-21.** Evidence for each:
`docs/research/m7-sources.md`.

| # | Question | Default |
|---|---|---|
| OI-17 | Build the Personio feed adapter? It is public with no login, and its `robots.txt` allows all; Personio's own help pages couldn't be read. It brings Inyad's 22 Casablanca postings. | **Yes.** If you find Personio terms against it, we remove it. |
| OI-18 | Use SmartRecruiters' Posting API? It's an official API that works without a key, documented for customers' career sites, but its `robots.txt` disallows every agent except LinkedInBot. It covers Devoteam (60 jobs in Morocco), Alten (165) and Delivery Hero (6). | **No.** Those companies stay manual-add (`collector add <link>`). |
| OI-19 | We Work Remotely, Himalayas, Working Nomads: their terms couldn't be read (403 or none found). | **Skip** until you or I can read their terms; Remotive and RemoteOK only. |
| OI-20 | "Since my last visit" in the Today view | The start of your last visit that's at least 30 minutes old, stored in the database. Reloading doesn't empty the list; "Mark all seen" resets it. No visit yet: the last 24 hours. |
| OI-21 | French evidence (for French postings) | Optional `text_fr` on each evidence line, `letter_fr` lines and `person.headline_fr`, all written and checked by you, never machine-translated. French drafts only when every selected line has French text; otherwise English, with the reason shown. |
| — | Dreamjob.ma RSS: allowed by its robots.txt, but mostly public-sector "concours" | **Skip.** |
