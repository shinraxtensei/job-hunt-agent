# M4: owner input

Five things only the owner can provide. Each has its file, its exact format and the story
that waits for it. Personal files live in `data/profile/` (git-ignored, never committed,
backed up by `make backup`). Nothing here is invented or filled in for you: where Claude
drafts something, you check every line before it is used, as with `skill_gaps.yaml`.

| # | What | File | Needed by | Blocks |
|---|---|---|---|---|
| OI-1 | Master CV as evidence lines (**draft path chosen**: Claude drafts, you check) | `data/profile/evidence.yaml` | M4-01 | M4-02, M4-03, M4-05 |
| OI-2 | Evidence for the 9 skills set to familiar (optional) | `data/profile/skill_map.yaml` | M4-01 | nothing |
| OI-3 | CV-variant label for the 36 eval postings | `agents/evals/variants.yaml` | M4-06 | M4-06's measurement |
| OI-4 | One-page CV layout (**defaults for now**) | `data/profile/cv_layout.yaml` | M4-02 | nothing |
| OI-5 | Review and approve the M4 stories | this PR | all | all |

## OI-1: Master CV as evidence lines

**What it is.** Everything a tailored CV may say. Tailoring selects, reorders and
rephrases these lines; it never adds a skill, number, title, employer, date or degree
that isn't here. Exact facts (titles, employers, dates, degrees) are printed character
for character.

**Two ways to provide it:**
- **Write it yourself** in the format below. Start from the committed
  `data/profile/evidence.example.yaml` (created in M4-01).
- **Or put your current Master CV** (any format: PDF, DOCX, Markdown) at
  `data/profile/master_cv.<ext>`. Claude drafts `evidence.yaml` from it, every line
  marked `checked: false`. You read each line, fix it, and set `checked: true`. The
  import refuses any unchecked line.

**Format.** One file. Keep the ids stable: drafts cite them. The values below are
placeholders, not your data.

```yaml
person:                      # the CV header, printed as written
  name: "Your Name"
  headline: "Backend Software Engineer"
  email: "you@example.com"
  phone: "+212 6 00 00 00 00"
  location: "Casablanca, Morocco"
  links: ["https://github.com/you", "https://linkedin.com/in/you"]

summary:                     # sentences a draft's summary may reuse (same shape as a line)
  - id: summary-years
    variants: [BE]
    skills: [Node.js]
    text: "Backend engineer with 3 years building Node.js services."
    checked: true

roles:                       # jobs, newest first
  - id: example              # short, unique, never reused
    employer: "Example Corp"           # exactly as on your contract
    title: "Backend Software Engineer" # exactly as on your contract
    start: 2023-05           # YYYY-MM
    end: present             # YYYY-MM or present
    location: "Casablanca, Morocco"
    lines:
      - id: example-billing  # unique across the file
        variants: [BE, PAY]  # which CV variants may use it: BE PAY DIST FS AI CONS
        skills: [Node.js, PostgreSQL]  # names from your skill map
        text: "Rebuilt the billing service's invoice job, cutting its run time from 40 to 6 minutes."
        source: "cv"         # optional: where the line came from (the draft fills it)
        checked: true        # you confirmed it is true as written

projects:                    # personal or school projects: `name` instead of employer/title
  - id: side-project
    name: "Example side project"
    stack: "TypeScript, PostgreSQL"   # optional, as printed; dates optional too
    start: 2025-01
    end: present
    lines:
      - id: side-project-api
        variants: [BE, FS]
        skills: [TypeScript, PostgreSQL]
        text: "Wrote a REST API and its Postgres schema for a small booking app."
        checked: true

education:
  - id: school
    school: "Example School"
    degree: "Example degree"   # exactly as on the diploma
    start: 2019-10
    end: 2022-07

certifications: []           # same shape as education: id, name, issuer, date (YYYY-MM)

languages:                   # printed as written
  - {name: Arabic, level: native}
  - {name: French, level: fluent}
  - {name: English, level: fluent}
```

**Rules the import checks** (M4-01). Each failure names its line:
- Every id is unique. Every `skills` name resolves in your skill map.
- Every number in a `text` stays exactly as you wrote it. A draft can only use numbers
  that appear in the lines it cites.
- A `learning` skill may be named only in a **project** line, stated as a fact ("Built
  a job collector in Go"). In a role line it is refused, because learning is not work
  experience.
- A line that names a `familiar` skill is allowed, but that skill is never presented as
  an achievement. It can appear in the skills section only.
- Dates are `YYYY-MM`, `YYYY` (when only the year is known) or `present`. Project dates are optional.
- `checked` must be `true`. The ids `person`, `summary` and `languages` are reserved.

## OI-2: Evidence for the 9 skills set to familiar (optional)

In M3 these went to `familiar` because they had no evidence: Observability,
Infrastructure as code, Ubuntu, Agile, Debian, Bash, Virtualization, Grafana, DevOps.

- **To keep one familiar:** do nothing. CVs then never present it as experience.
- **To make one proven:** edit its entry in `data/profile/skill_map.yaml`, then add or
  tag a line in `evidence.yaml` that shows it:

```yaml
  - name: Grafana
    level: proven
    evidence: "<employer or project>: <what you built or ran with it>"
```

Write the `evidence` text as a fact someone could check. The skill map refuses
`proven` without it.

## OI-3: CV-variant label for the 36 eval postings

`decide()` picks a CV variant per lead. In the 509-lead run it chose DIST 173 times,
which looks skewed. To measure that, M4-06 writes `agents/evals/variants.yaml` with the
36 postings and an empty answer each. Fill in the answers without looking at the
model's choice; the file doesn't show it.

```yaml
p3501:                  # SumUp — Backend Engineer - Run & Grow
  variant: "?"          # one of: BE PAY DIST FS AI CONS
  note: ""              # optional, only when it's a close call ("PAY or BE")
p4067:                  # Monzo — Senior Android Engineer
  variant: "?"
  note: ""
```

The variants:

| Variant | Use for |
|---|---|
| BE | Backend engineering |
| PAY | Payments and fintech |
| DIST | Distributed systems and infrastructure |
| FS | Full-stack |
| AI | AI, LLM and ML engineering |
| CONS | Consulting or client-facing engineering |

`variants.yaml` is not personal, so it is committed, like the other labels.

## OI-4: One-page CV layout

What a draft must fit on one page. M4 drafts the content; M5 turns it into a PDF.

```yaml
# data/profile/cv_layout.yaml
page: A4                       # A4 | Letter
sections: [summary, experience, projects, skills, education, languages]  # order; leave one out to drop it
summary: true                  # 1–2 sentences at the top, built from evidence lines only
max_roles: 3                   # newest first; older roles are left out
max_bullets_per_role: 4
max_projects: 2
max_bullets_per_project: 2
skills_section: grouped        # grouped (Backend: …, Data: …) | flat
skills_max: 14                 # proven first, then familiar
reference: data/profile/cv_reference.pdf   # optional: your current one-page CV, for M5's look
```

If you'd rather not decide now, skip this file. M4-02 then uses the values above as
defaults, and you approve or change them on the first draft (M4-05).

## OI-5: Review the M4 stories

Read `M4-01` to `M4-06` in this PR. Approve them, or tell me what to change, before any
code starts.
