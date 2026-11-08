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
"""

import logging
from dataclasses import dataclass
from pathlib import Path
from typing import Any, cast

from psycopg.types.json import Jsonb
from psycopg_pool import ConnectionPool
from pydantic import BaseModel, ConfigDict
from ruamel.yaml import YAML

from jobhunt_agents.coverage import Coverage, coverage
from jobhunt_agents.drafts import DraftContent
from jobhunt_agents.evidence import Bank, EvidenceWatcher, Line
from jobhunt_agents.extraction import Extraction, drop_company
from jobhunt_agents.model import MODEL, ModelClient, Prompt
from jobhunt_agents.profile import Profile, ProfileWatcher
from jobhunt_agents.queue import Handler, Item, ItemError, NeedsReview
from jobhunt_agents.validator import validate

log = logging.getLogger(__name__)

AGENT = "tailor"
PROMPT_VERSION = 1
MAX_TOKENS = 2000


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
    def __init__(self, client: ModelClient, prompt: Prompt | None = None) -> None:
        self.client = client
        self.prompt = prompt or Prompt.load(AGENT, PROMPT_VERSION, cacheable=False)

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
SELECT p.content_hash, e.data, c.name, coalesce(s.cv_variant, 'BE')
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
UPDATE drafts SET status = %s, content_hash = %s, variant = %s, content = %s, coverage = %s,
    findings = %s, evidence_version = %s, profile_version = %s, prompt_version = %s, model = %s,
    made_at = now()
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
        content_hash, data, company, variant = lead
        layout = Layout.load(layout_path) if layout_path else Layout()
        cov = coverage(
            drop_company(Extraction.model_validate(data), company), profile, bank, variant
        )
        sel = select(bank, cov, layout)
        try:
            content = tailorer.draft(prompt_data(cov, sel, layout, profile), item.posting_id)
        except NeedsReview as e:
            with pool.connection() as conn:
                conn.execute(_FAIL, (f"{type(e).__name__}: {e}"[:500], item.posting_id))
            raise
        rendered = render(content, bank, sel, layout, profile)
        report = validate(checked(rendered), bank, profile, coverage=cov)
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
                    Jsonb(rendered),
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
