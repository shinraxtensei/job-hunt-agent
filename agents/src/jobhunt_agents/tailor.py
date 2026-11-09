# ruamel.yaml is untyped (ADR-0019).
# pyright: reportUnknownMemberType=false, reportUnknownVariableType=false
"""The tailoring agent (M4-02, ADR-0025): a one-page CV draft for one lead, from the
evidence bank only (evidence-bank-guardrails).

1. Evidence is selected in code: lines covering the lead's skills first, then lines
   tagged with its CV variant, within the layout's limits.
2. The model gets only those lines and the posting's extracted skill names (untrusted,
   in the escaped data block). It never sees the posting's text, the free-text profile
   or the whole bank.
3. Code fills in the header and the exact facts (titles, employers, dates, degrees).
4. The validator (M4-04) checks every sentence; the draft is stored blocked or
   ready_for_review. Nothing is ever sent.

The cover letter (M5-01) is made in the same draft, from a second call with the same
selected lines plus the bank's `letter` lines. Code adds the greeting, the opening (the
lead's job title and company), the sign-off and the name; the model writes none of them.

French postings (M7-02, OI-21) get a French draft from the owner's own French text
(`text_fr`, never machine-translated) when every line the draft may use has one; otherwise
the draft is English and says which lines lack it.
"""

import logging
from dataclasses import dataclass
from pathlib import Path
from typing import Any, cast

from psycopg.types.json import Jsonb
from psycopg_pool import ConnectionPool
from pydantic import BaseModel, ConfigDict
from ruamel.yaml import YAML

from jobhunt_agents.checker import Checker, sentences
from jobhunt_agents.coverage import Coverage, coverage, suspicious
from jobhunt_agents.drafts import DraftContent, LetterContent
from jobhunt_agents.evidence import Bank, EvidenceWatcher, Line
from jobhunt_agents.extraction import Extraction, drop_company
from jobhunt_agents.model import MODEL, ModelClient, Prompt
from jobhunt_agents.profile import Profile, ProfileWatcher
from jobhunt_agents.queue import Handler, Item, ItemError, NeedsReview
from jobhunt_agents.text import language, posting_text
from jobhunt_agents.validator import Finding, Report, validate

log = logging.getLogger(__name__)

AGENT = "tailor"
PROMPT_VERSION = 1
MAX_TOKENS = 2000
LETTER_AGENT = "letter"
LETTER_PROMPT_VERSION = 3  # v2: the lines' language (M7-02); v3: generic examples (M8-03)
MAX_WORDS = 250  # the whole letter, greeting and sign-off included (PRD)
GREETING = "Dear Hiring Team,"  # OI-10
SIGN_OFF = "Best regards,"
# greeting and sign-off by language, written by code (M7-02)
FIXED = {"en": (GREETING, SIGN_OFF), "fr": ("Madame, Monsieur,", "Cordialement,")}


class Layout(BaseModel):
    """OI-4: what fits one page (data/profile/cv_layout.yaml; these are the defaults)."""

    model_config = ConfigDict(frozen=True, extra="forbid")
    page: str = "A4"
    sections: tuple[str, ...] = (
        "summary",
        "experience",
        "projects",
        "skills",
        "education",
        "languages",
    )
    summary: bool = True
    max_roles: int = 3
    max_bullets_per_role: int = 4
    max_projects: int = 2
    max_bullets_per_project: int = 2
    skills_section: str = "grouped"
    skills_max: int = 14
    reference: str | None = None

    @classmethod
    def load(cls, path: Path) -> Layout:
        if not path.is_file():
            return cls()
        raw = cast(dict[str, Any], YAML(typ="safe").load(path.read_text()) or {})
        return cls.model_validate(raw)


@dataclass(frozen=True)
class Picked:
    id: str  # role or project id
    limit: int  # bullets the model may write for it
    lines: tuple[Line, ...]  # candidates, best first


@dataclass(frozen=True)
class Selection:
    summary: tuple[Line, ...]
    roles: tuple[Picked, ...]
    projects: tuple[Picked, ...]


def select(bank: Bank, cov: Coverage, layout: Layout) -> Selection:
    """Candidates per section: lines covering the lead's skills (covered or listed) first,
    then lines tagged with its variant, then the rest; one more than the bullets allowed,
    so the model can choose. Suspicious names never count (they cover nothing)."""
    useful: dict[str, int] = {}
    for s in cov.skills:
        if s.status != "gap":
            for line_id in s.lines:
                useful[line_id] = useful.get(line_id, 0) + 1

    def rank(lines: tuple[Line, ...]) -> list[Line]:
        return sorted(lines, key=lambda x: (-useful.get(x.id, 0), cov.variant not in x.variants))

    roles = tuple(
        Picked(
            r.id,
            layout.max_bullets_per_role,
            tuple(rank(r.lines)[: layout.max_bullets_per_role + 1]),
        )
        for r in bank.roles[: layout.max_roles]
        if r.lines
    )
    projects = sorted(
        bank.projects, key=lambda p: -max((useful.get(x.id, 0) for x in p.lines), default=0)
    )
    picked = tuple(
        Picked(
            p.id,
            layout.max_bullets_per_project,
            tuple(rank(p.lines)[: layout.max_bullets_per_project + 1]),
        )
        for p in projects[: layout.max_projects]
        if p.lines
    )
    summary = tuple(rank(bank.summary)[:3]) if layout.summary else ()
    return Selection(summary=summary, roles=roles, projects=picked)


def listable(cov: Coverage, sel: Selection, profile: Profile) -> list[str]:
    """Skills the skills section may name: proven or familiar ones the job asks for
    (covered first, then listed), then those the selected lines name. Never a gap."""
    out: list[str] = []
    for status in ("covered", "listed"):
        out += [s.skill for s in cov.skills if s.status == status and s.skill]
    for line in (*sel.summary, *(x for p in (*sel.roles, *sel.projects) for x in p.lines)):
        out += [r.skill for r in line.skills if r.level in ("proven", "familiar")]
    seen = {s.name for s in profile.skill_map.skills if s.level in ("proven", "familiar")}
    return [x for x in dict.fromkeys(out) if x in seen]


def prompt_data(cov: Coverage, sel: Selection, layout: Layout, profile: Profile) -> str:
    """What the model reads: the lead's skills with their status, and the selected lines."""
    out = [f"CV variant: {cov.variant}", "", "Job skills:"]
    out += [f"- {s.name}: {s.status}" + (f" ({s.why})" if s.why else "") for s in cov.skills]
    out += ["", "Summary lines:"] + [f"- [{x.id}] {x.text}" for x in sel.summary]
    for kind, picked in (("Role", sel.roles), ("Project", sel.projects)):
        for p in picked:
            out += ["", f"{kind} {p.id} (at most {p.limit} bullets):"]
            out += [f"- [{x.id}] {x.text}" for x in p.lines]
    out += [
        "",
        f"Skills section: at most {layout.skills_max} names, only from: "
        + ", ".join(listable(cov, sel, profile)),
    ]
    return "\n".join(out)


class Tailorer:
    def __init__(
        self,
        client: ModelClient,
        prompt: Prompt | None = None,
        letter_prompt: Prompt | None = None,
        checker: Checker | None = None,
    ) -> None:
        self.client = client
        self.checker = checker or Checker(client)  # every draft goes through it (M5-02)
        self.prompt = prompt or Prompt.load(AGENT, PROMPT_VERSION, cacheable=False)
        self.letter_prompt = letter_prompt or Prompt.load(
            LETTER_AGENT, LETTER_PROMPT_VERSION, cacheable=False
        )

    def letter(self, data: str, posting_id: int | None = None) -> LetterContent:
        return self.client.structured(
            agent=LETTER_AGENT,
            prompt=self.letter_prompt,
            data=data,
            output=LetterContent,
            max_tokens=MAX_TOKENS,
            posting_id=posting_id,
            thinking=False,
        )

    def draft(self, data: str, posting_id: int | None = None) -> DraftContent:
        return self.client.structured(
            agent=AGENT,
            prompt=self.prompt,
            data=data,
            output=DraftContent,
            max_tokens=MAX_TOKENS,
            posting_id=posting_id,
            thinking=False,  # a structured rewrite; thinking would eat the budget
        )


def render(
    content: DraftContent, bank: Bank, sel: Selection, layout: Layout, profile: Profile
) -> dict[str, Any]:
    """The stored draft: the header and exact facts from the bank (never the model's),
    the model's sentences within the layout's limits, in the bank's order. The skills
    section keeps only proven and familiar skills: dropping adds nothing (rule 1)."""
    allowed = {p.id: p.limit for p in (*sel.roles, *sel.projects)}
    by_id = {s.id: s.bullets for s in (*content.roles, *content.projects) if s.id in allowed}
    roles = [
        r.model_dump(exclude={"lines"})
        | {"bullets": [b.model_dump() for b in by_id[r.id][: allowed[r.id]]]}
        for r in bank.roles
        if r.id in by_id
    ]
    projects = [
        p.model_dump(exclude={"lines"})
        | {"bullets": [b.model_dump() for b in by_id[p.id][: allowed[p.id]]]}
        for p in bank.projects
        if p.id in by_id
    ]
    return {
        "person": bank.person.model_dump(),
        "summary": [b.model_dump() for b in content.summary[:2]],
        "roles": roles,
        "projects": projects,
        "skills": [
            s
            for s in content.skills
            if (m := profile.skill_map.match(s)) is not None and m.level != "learning"
        ][: layout.skills_max],
        "education": [e.model_dump() for e in bank.education],
        "certifications": [c.model_dump() for c in bank.certifications],
        "languages": [x.model_dump() for x in bank.languages],
        "layout": layout.model_dump(),
    }


def checked(rendered: dict[str, Any]) -> DraftContent:
    """The rendered draft's sentences, as validated."""
    return DraftContent.model_validate(
        {
            "summary": rendered["summary"],
            "roles": [{"id": r["id"], "bullets": r["bullets"]} for r in rendered["roles"]],
            "projects": [{"id": p["id"], "bullets": p["bullets"]} for p in rendered["projects"]],
            "skills": rendered["skills"],
        }
    )


def opening(title: str, company: str, lang: str = "en") -> str:
    """The letter's first sentence, by code. The title comes from the posting (untrusted):
    one line, and replaced when it looks like an instruction (suspicious() also catches
    anything over 60 characters)."""
    title, company = " ".join(title.split()), " ".join(company.split())[:80]
    known = bool(title) and not suspicious(title)
    if lang == "fr":
        if not known:
            return f"Je vous écris pour postuler à un poste ouvert chez {company}."
        return f"Je vous écris pour postuler au poste de {title} chez {company}."
    if not known:
        return f"I am writing to apply for the open role at {company}."
    return f"I am writing to apply for the {title} role at {company}."


def language_plan(posting_lang: str, bank: Bank, sel: Selection) -> tuple[str, Bank, str]:
    """The draft's language, the bank to draft from and a note. French only when every line
    the CV or letter may use (and the headline) has the owner's French text (OI-21)."""
    if posting_lang != "fr":
        return "en", bank, ""
    used = [*sel.summary, *(x for p in (*sel.roles, *sel.projects) for x in p.lines)]
    missing = [x.id for x in (*used, *bank.letter) if not x.text_fr]
    if bank.person.headline and not bank.person.headline_fr:
        missing.append("headline")
    if missing:
        return "en", bank, f"English: no French text for {', '.join(dict.fromkeys(missing))}."
    return "fr", bank.french(), ""


def for_letter(sel: Selection) -> Selection:
    """The selection without lines naming a learning skill: a project line may state one
    as fact in the CV, but the letter would read it as experience (guardrail rule 3)."""

    def keep(lines: tuple[Line, ...]) -> tuple[Line, ...]:
        return tuple(x for x in lines if all(r.level != "learning" for r in x.skills))

    def picked(ps: tuple[Picked, ...]) -> tuple[Picked, ...]:
        return tuple(Picked(p.id, p.limit, keep(p.lines)) for p in ps if keep(p.lines))

    return Selection(keep(sel.summary), picked(sel.roles), picked(sel.projects))


def letter_data(base: str, bank: Bank, budget: int) -> str:
    """The CV's prompt data, the letter lines and the word budget for the sentences."""
    out = [base, "", "Roles (exact facts):"]
    out += [f"- {r.id}: {r.title} at {r.employer}" for r in bank.roles]
    out += ["", "Letter lines (letter only):"]
    out += [f"- [{x.id}] {x.text}" for x in bank.letter]
    out += ["", f"Words: at most {budget} in all your sentences together."]
    return "\n".join(out)


def _words(text: str) -> int:
    return len(text.split())


def render_letter(
    content: LetterContent, bank: Bank, title: str, company: str, lang: str = "en"
) -> dict[str, Any]:
    first = opening(title, company, lang)
    greeting, sign_off = FIXED[lang]
    paragraphs = [[b.model_dump() for b in p.sentences] for p in content.paragraphs]
    fixed = (greeting, first, sign_off, bank.person.name)
    words = sum(map(_words, fixed)) + sum(_words(b["text"]) for p in paragraphs for b in p)
    return {
        "language": lang,
        "greeting": greeting,
        "opening": first,
        "paragraphs": paragraphs,
        "closing": sign_off,
        "name": bank.person.name,
        "words": words,
    }


@dataclass(frozen=True)
class Made:
    content: dict[str, Any]
    letter: dict[str, Any]
    report: Report


def make(  # noqa: PLR0913 - the lead's parts, kept apart for the tests
    tailorer: Tailorer,
    bank: Bank,
    profile: Profile,
    cov: Coverage,
    layout: Layout,
    title: str,
    company: str,
    *,
    posting_id: int | None = None,
    posting_lang: str = "en",
) -> Made:
    """CV and letter for one lead, validated together, then, if the code checks pass, by
    the independent claim checker (M5-02). NeedsReview propagates. A French draft is
    drafted, validated and checked against the French bank: the same ids and numbers."""
    lang, bank, lang_note = language_plan(posting_lang, bank, select(bank, cov, layout))
    sel = select(bank, cov, layout)
    data = prompt_data(cov, sel, layout, profile)
    content = render(tailorer.draft(data, posting_id), bank, sel, layout, profile)
    content |= {"language": lang, "language_note": lang_note}
    greeting, sign_off = FIXED[lang]
    first = opening(title, company, lang)
    fixed = sum(map(_words, (greeting, first, sign_off, bank.person.name)))
    budget = MAX_WORDS - fixed
    ldata = letter_data(prompt_data(cov, for_letter(sel), layout, profile), bank, budget)
    raw = tailorer.letter(ldata, posting_id)
    letter = render_letter(raw, bank, title, company, lang)
    if letter["words"] > MAX_WORDS:  # one retry, told how long it was
        note = f"\nPrevious attempt: {letter['words'] - fixed} words, over the budget."
        raw = tailorer.letter(ldata + note, posting_id)
        letter = render_letter(raw, bank, title, company, lang)
    letter["prompt_version"] = tailorer.letter_prompt.version
    report = validate(checked(content), bank, profile, coverage=cov, letter=raw)
    if letter["words"] > MAX_WORDS:
        report.violations.append(
            Finding("too_long", "letter", "", None, f"{letter['words']} words, at most {MAX_WORDS}")
        )
    if not report.violations:  # the checker judges only what the code passed
        report.violations += tailorer.checker.check(sentences(content, letter), bank, posting_id)
    return Made(content, letter, report)


# Requested drafts get a tailor item; a done item is re-queued for a new request.
_QUEUE = """
INSERT INTO work_items (posting_id, kind, content_hash)
SELECT DISTINCT ON (d.posting_id) d.posting_id, 'tailor', p.content_hash
FROM drafts d JOIN postings p ON p.id = d.posting_id
WHERE d.status = 'requested'
ON CONFLICT (posting_id, kind) DO UPDATE SET
    state = 'pending', attempts = 0, last_error = NULL, run_after = now(),
    content_hash = excluded.content_hash, updated_at = now()
WHERE work_items.state IN ('done', 'failed')
"""

_LEAD = """
SELECT p.content_hash, e.data, c.name, coalesce(s.cv_variant, 'BE'), j.title, p.source,
    p.raw_payload
FROM postings p
JOIN jobs j ON j.posting_id = p.id
JOIN companies c ON c.id = j.company_id
JOIN scores s ON s.posting_id = p.id
JOIN extractions e ON e.id = s.extraction_id
WHERE p.id = %s
"""

_REQUESTED = """
SELECT id FROM drafts WHERE posting_id = %s AND status = 'requested' ORDER BY id DESC LIMIT 1
"""

_STORE = """
UPDATE drafts SET status = %s, content_hash = %s, variant = %s, content = %s, letter = %s,
    coverage = %s, findings = %s, evidence_version = %s, profile_version = %s,
    prompt_version = %s, model = %s, made_at = now()
WHERE posting_id = %s AND status = 'requested' AND id <= %s
"""

_FAIL = """
UPDATE drafts SET status = 'failed', error = %s, made_at = now()
WHERE posting_id = %s AND status = 'requested'
"""


def queue_drafts(pool: ConnectionPool) -> None:
    with pool.connection() as conn:
        conn.execute(_QUEUE)


def handler(
    pool: ConnectionPool,
    tailorer: Tailorer,
    profiles: ProfileWatcher,
    evidence: EvidenceWatcher,
    layout_path: Path | None = None,
) -> Handler:
    """The `tailor` work-item handler: makes the lead's requested draft. Malformed output
    twice fails the draft and the item (needs_review); nothing is claimed then."""

    def run(item: Item) -> None:
        profile = profiles.current()
        bank = evidence.current(profile)
        if bank is None:
            raise ItemError("no evidence bank: data/profile/evidence.yaml is missing or invalid")
        with pool.connection() as conn:
            req = conn.execute(_REQUESTED, (item.posting_id,)).fetchone()
            lead = conn.execute(_LEAD, (item.posting_id,)).fetchone()
        if req is None:
            return  # nothing requested (already made)
        if lead is None:
            raise ItemError("no score or extraction for this posting")
        content_hash, data, company, variant, title, source, payload = lead
        # only code reads the posting's text, to tell its language; no model sees it
        lang = language(posting_text(source, payload, title=title, location=""))
        layout = Layout.load(layout_path) if layout_path else Layout()
        cov = coverage(
            drop_company(Extraction.model_validate(data), company), profile, bank, variant
        )
        try:
            made = make(
                tailorer,
                bank,
                profile,
                cov,
                layout,
                title,
                company,
                posting_id=item.posting_id,
                posting_lang=lang,
            )
        except NeedsReview as e:
            with pool.connection() as conn:
                conn.execute(_FAIL, (f"{type(e).__name__}: {e}"[:500], item.posting_id))
            raise
        report = made.report
        findings = {
            "violations": [f.__dict__ for f in report.violations],
            "warnings": [f.__dict__ for f in report.warnings],
        }
        with pool.connection() as conn:
            conn.execute(
                _STORE,
                (
                    report.status,
                    content_hash,
                    variant,
                    Jsonb(made.content),
                    Jsonb(made.letter),
                    Jsonb(cov.model_dump()),
                    Jsonb(findings),
                    bank.version,
                    profile.version,
                    tailorer.prompt.version,
                    MODEL,
                    item.posting_id,
                    req[0],
                ),
            )
        log.info("draft made", extra={"item_id": item.id, "outcome": report.status})

    return run
