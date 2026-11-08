# M5 owner input

Each item has a default. Reply "defaults" to take them all.

**Answered 2026-11-08: defaults for all**, with two notes recorded in M5-01 (French
evidence for M7-04) and M5-03 (embedded TTF for accents).

| # | Question | Default |
|---|---|---|
| OI-6 | What may a letter say beyond evidence lines? | Only the company name and job title (inserted by code), plus checked lines you write in a new `letter` section of `evidence.yaml` (format below). No invented motivation or "why this company". |
| OI-7 | Letter language | English always, even for French postings (no translation of evidence). |
| OI-8 | PDF library | `fpdf2` (pure Python, no native code); `pypdf` as a test-only dependency for the ATS text check. |
| OI-9 | Hand edits before export | None in M5: change `evidence.yaml` and make a new draft. Every exported word then stays checked. |
| OI-10 | Greeting and sign-off | "Dear Hiring Team," … "Best regards," + your name. |
| — | M3-08 screenshots with real reasons in git history (repo is private) | Leave until the repo goes public (M8), then rewrite history (force-push) with your OK. |
| — | LinkedIn URL in the CV header | None until you add `links` to `evidence.yaml`. |

## `letter` section format (OI-6, optional)

```yaml
letter:
  - id: letter-availability
    text: Available to start within one month.
    checked: true
  - id: letter-relocation
    text: Open to relocating within Europe.
    checked: true
```

Lines here may be used only in the letter, never in the CV.
