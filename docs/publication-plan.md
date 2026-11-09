# Before the repository goes public (M8-03)

Scanned 2026-11-09, all refs (242 commits). **Nothing below has been done to the
history, and the repository is still private.** Rewriting history and changing visibility
are the owner's decisions (irreversible). Personal values are never written here.

## How it was scanned

- **gitleaks** v8.18.4 (secret patterns) over `--all` history: 228 commits, **no leaks**.
- `.env` values are matched only for secret-like keys (key, password, secret, token, URL):
  slugs and ports are not personal.
- **`agents/tools/history_scan.py`**: loads 346 of the owner's values at run time (the
  evidence bank's person block, every line's text and French text, employer, school and
  project names, every `.env` value, the 300 latest real fit reasons). It searches every
  blob ever committed and prints only labels and paths. Images are listed for a person to
  look at.
- Re-run both with `make history-scan`.

## Findings

| # | What | Where | Verdict |
|---|---|---|---|
| H1 | Screenshots of the live tracker with real fit reasons, scores and gaps against the owner's profile | `docs/design/screens/m3-08/` (8 files; commits b6c080e, 3bf79c5, bc01dfa, ea9b6d8) | **personal: remove from history** |
| H2 | Real collected leads with the owner's filter settings and lead counts (no scores): screenshots, the M2-06 mockup snapshot and the mockups built from it | `docs/design/mockups/` (snapshot.json, 3 HTML mockups, 15 shots), `docs/design/polish/before/`, `polish/after/` (8), `docs/design/screens/{dark,light}-*.png` (4) | public postings, but reveal the search: **remove (recommended)** |
| H3 | A product name from the owner's work history, used as an example | `agents/src/jobhunt_agents/prompts/letter/v1.md`, `v2.md`, `docs/stories/M5-02-claim-checker.md` | **replace** in every version |
| H5 | The owner's blind fit labels on public postings (the eval set) | `agents/evals/labels/` | **owner's choice**: they show how the evals were built; keep by default |
| H4 | The owner's personal email as the git author and committer of 166 commits | commit metadata | **owner's choice**: map to the GitHub noreply address |
| — | The owner's GitHub profile link | User-Agent and tests (`internal/source/fetch/fetch.go`, …) | the repository's own URL: fine |
| — | `.env.example` | committed since M0 | template only; no `.env` value found in any blob |
| — | Screenshots in `docs/design/screens/m4-05/`, `m7-00/`, `docs/screens/` | — | fixture or demo data: fine |
| — | evidence text, name, email, phone, employers, schools, real reasons, `.env` values | any text blob | **not found** |

Already done in the tree (PR for M8-03): H1 and H2 files are deleted from `master`, the
M5-02 story no longer names the product, and the README uses demo screenshots only. The
letter prompts (H3) still name it: changing a prompt needs `make evals-tailor` and
`make evals-checker`, so that happens with the owner's go-ahead (a letter prompt v3).

## The rewrite plan (for the owner to run, or to approve)

GitHub keeps the old commits reachable through pull-request refs (`refs/pull/*`), which a
force-push can't remove. So the clean way is a **new public repository** made from the
rewritten history, with the current private one kept as it is (or deleted later).

1. **Back up:** `git clone --mirror git@github.com:shinraxtensei/job-hunter.git job-hunter-backup.git`
   and keep it offline.
2. **Install** git-filter-repo (`brew install git-filter-repo`).
3. **Fresh clone:** `git clone --no-local --mirror job-hunter-backup.git job-hunter-public.git`
   then `cd job-hunter-public.git`.
4. **Drop the screenshots (H1, H2):**
   ```bash
   git filter-repo --invert-paths \
     --path docs/design/screens/m3-08 --path docs/design/mockups \
     --path docs/design/polish \
     --path docs/design/screens/dark-1280.png --path docs/design/screens/dark-375.png \
     --path docs/design/screens/light-1280.png --path docs/design/screens/light-375.png
   ```
5. **Replace the name (H3):** a file `replace.txt` with one line,
   `regex:(?i)<the product name>==>Acme`, then
   `git filter-repo --replace-text replace.txt`.
6. **Author address (H4, if chosen):** a file `mailmap` with
   `Anass <48833654+shinraxtensei@users.noreply.github.com> <the personal email>`, then
   `git filter-repo --mailmap mailmap`.
7. **Check:** in a working clone of the result, `make history-scan` must report no
   personal values and no H1/H2 paths; `make check` must pass.
8. **Publish:** create the new repository, push `master` and tags, set branch protection
   and CI secrets as on the old one, then make it public.
9. **Afterwards:** every clone is re-cloned; old commit links (Linear, PR descriptions)
   point to the private repository.
