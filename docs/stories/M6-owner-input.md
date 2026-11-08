# M6 owner input

Each item has a default. Reply "defaults" to take them all.

**Answered 2026-11-08:** defaults for OI-11 to OI-14; OI-15 and OI-16 changed as below.

| # | Question | Default |
|---|---|---|
| OI-11 | Which statuses change on their own? | Only `drafted`: set when you approve a draft, if the lead is still `new` or `shortlisted`. Marking a document sent does **not** set `applied`; the page offers it in one click. Everything else is yours. |
| OI-12 | When does an application exist? | As soon as a lead leaves `new`, for its duplicate group (one application per group). |
| OI-13 | Board columns | shortlisted · drafted · applied · replied · interviewing · offer; rejected, closed and skipped folded into a "Done" column; `new` stays in the leads table. |
| OI-14 | Contacts (other people's personal data) | Stored only in the local database and its backups; never sent to a model, never logged, never exported. Fields: name (required), role, email, LinkedIn URL, phone. |
| OI-15 | Follow-up delay and message | **Answered:** 7 days after `applied`, snooze 3 or 7 days; a fixed template filled in by code, no model: "Hello {name}," (or "Hello," with no contact name) / "I applied for the {title} role at {company} on {date} and wanted to follow up. I'm still very interested and happy to share anything else that would help." / "Best regards," / your name. |
| OI-16 | Skip reasons list | **Answered:** location · seniority · stack · salary · company · role type · visa / relocation · language · not interested · other (free text). |
| — | Order | M6-01 → M6-05 → M6-03 → M6-04 → M6-02 → M6-06 (the board can slip, as the PRD allows). |
