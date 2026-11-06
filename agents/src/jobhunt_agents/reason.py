"""The reason agent (M3-06 AC2, ADR-0021): a reason of at most two sentences and the gaps,
from the coded breakdown only, plus the CV variant through decide().

The model never sees the posting's text: only factor values, the profile's level for each
skill the extraction named, and (for decide) the title and skill names. Its output can't
change the score; `check` rejects gaps outside the breakdown and any score claim, and the
model client retries once, then the item goes to needs_review.
"""

import re
from typing import Any, ClassVar

from psycopg.types.json import Jsonb
from psycopg_pool import ConnectionPool
from pydantic import BaseModel, ConfigDict, Field, model_validator

from jobhunt_agents.aliases import canon
from jobhunt_agents.extraction import Extraction, drop_company
from jobhunt_agents.model import MODEL, ModelClient, Prompt
from jobhunt_agents.queue import Handler, Item, ItemError
from jobhunt_agents.score import Config, load_config

AGENT = "reason"
PROMPT_VERSION = 2  # bump with a new prompts/reason/vN.md; leads regenerate when opened
MAX_TOKENS = 400
VARIANTS = ["BE", "PAY", "DIST", "FS", "AI", "CONS"]
VARIANT_QUESTION = (
    "Which CV variant fits this job best? BE: backend engineering. PAY: payments and "
    "fintech. DIST: distributed systems and infrastructure. FS: full-stack. AI: AI, LLM "
    "and ML engineering. CONS: consulting or client-facing engineering."
)
_LABELS = {
    "required": "required skills",
    "title": "title match",
    "seniority": "seniority and years",
    "location": "location, remote, visa",
    "optional": "optional skills",
}
_SENTENCE_END = re.compile(r"(?<=[.!?])\s+")  # "Node.js" and "0.8" are not ends
# The score is the code's: no numbers of points, percentages, ratings or obeyed asks.
_SCORE_CLAIM = re.compile(
    r"\d+\s*(/\s*100|%|points?|pts)|\bscor(e|ed|es|ing)\b|\brat(e|ed|ing)\b|perfect fit"
    r"|\binstructions?\b",
    re.IGNORECASE,
)


# A clause that stops at a contrast or negation: "Your SQL is proven, but Rust is missing"
# says nothing about owning Rust.
_STOP = (
    r"but|while|though|although|whereas|however|not|no|missing|lack\w*"
    r"|asks?|needs?|requir\w*|wants?|weak\w*|gaps?"
)
_CLAUSE = rf"(?:(?!\b(?:{_STOP})\b|n't)[^.;:]){{0,60}}?"
# Said right after the name, these deny it: "EKS is missing", "Go, which you're learning".
_DENIED = re.compile(
    r"^[^.;:]{0,40}?\b(?:missing|learning|lack\w*|not|no)\b|^[^.;:]{0,40}?n't", re.I
)


def _owned(name: str) -> re.Pattern[str]:
    """ "your … X" or "you've … X" in one clause: X said to be the owner's. "your skill
    map" and "your skills" introduce what the owner lacks, so they don't count."""
    n = re.escape(name)
    return re.compile(
        rf"\b(?:your(?!\s+(?:\w+\s+)?(?:skill\s+map|skills?|profile|targets?))|you'?ve|you\s+have|you\s+use[ds]?)"
        rf"\b{_CLAUSE}(?<!\w){n}(?!\w)",
        re.IGNORECASE,
    )


def check(reason: str, gaps: list[str], allowed: dict[str, str]) -> None:
    """Raises ValueError unless the reason has at most two sentences and no score claim,
    and every gap is a skill the breakdown has below proven (allowed: name -> level)."""
    if len([s for s in _SENTENCE_END.split(reason.strip()) if s.strip()]) > 2:
        raise ValueError("reason: more than two sentences")
    if _SCORE_CLAIM.search(reason):
        raise ValueError("reason: states or follows a score")
    for name, level in allowed.items():
        # Golden rule 3: a missing or learning skill is never the owner's experience.
        if level not in ("missing", "learning"):
            continue
        for m in _owned(name).finditer(reason):
            if not _DENIED.search(reason[m.end() :]):
                raise ValueError("reason: presents a missing or learning skill as experience")
    names = {canon(n) for n in allowed}
    for g in gaps:
        if canon(g) not in names:
            raise ValueError("gaps: a skill not below proven in the breakdown")


class Reason(BaseModel):
    """Two one-sentence fields, so the reason has at most two sentences by construction."""

    model_config = ConfigDict(frozen=True, extra="forbid")
    best: str = Field(
        min_length=1, max_length=300, description="Exactly one sentence: what fits best."
    )
    worst: str = Field(
        min_length=1, max_length=300, description="Exactly one sentence: what fits worst."
    )
    gaps: list[str]  # most important first; the first five are kept

    allowed: ClassVar[dict[str, str]] = {}

    @property
    def reason(self) -> str:
        return f"{self.best.strip()} {self.worst.strip()}"

    @model_validator(mode="after")
    def _rules(self) -> Reason:
        for part in (self.best, self.worst):
            if len([x for x in _SENTENCE_END.split(part.strip()) if x.strip()]) != 1:
                raise ValueError("best and worst: one sentence each")
        check(self.reason, self.gaps, self.allowed)
        return self


def _output(allowed: dict[str, str]) -> type[Reason]:
    """The Reason schema for one breakdown: gaps checked against its skills."""
    return type("Reason", (Reason,), {"allowed": allowed})


def breakdown(factors: dict[str, Any], not_enough_info: bool, weights: dict[str, float]) -> str:
    """What the model reads: factor values, notes and skill levels. No posting text."""
    lines = ["NOT ENOUGH INFO: the posting names no required skills."] if not_enough_info else []
    for key, label in _LABELS.items():
        f = factors[key]
        note = f" ({f['detail']})" if f.get("detail") else ""
        lines.append(f"{label} (weight {weights[key]:g}): {f['value']:.2f}{note}")
        lines += [f"- {m['name']}: {m['level']}" for m in f.get("matches", ())]
    return "\n".join(lines)


def below_proven(factors: dict[str, Any]) -> dict[str, str]:
    return {
        m["name"]: m["level"]
        for k in ("required", "optional")
        for m in factors[k].get("matches", ())
        if m["level"] != "proven"
    }


class Reasoner:
    def __init__(self, client: ModelClient, prompt: Prompt | None = None) -> None:
        self.client = client
        self.prompt = prompt or Prompt.load(AGENT, PROMPT_VERSION, cacheable=False)  # short

    def reason(
        self,
        factors: dict[str, Any],
        not_enough_info: bool,
        weights: dict[str, float],
        posting_id: int | None = None,
    ) -> Reason:
        return self.client.structured(
            agent=AGENT,
            prompt=self.prompt,
            data=breakdown(factors, not_enough_info, weights),
            output=_output(below_proven(factors)),
            max_tokens=MAX_TOKENS,
            posting_id=posting_id,
            thinking=False,  # a short answer; thinking used whole budgets in evals
        )

    def variant(self, title: str, extraction: dict[str, Any], posting_id: int | None) -> str:
        context = (
            f"Title: {title}\nRequired skills: {', '.join(extraction['required_skills'])}\n"
            f"Optional skills: {', '.join(extraction['optional_skills'])}"
        )
        return self.client.decide(VARIANT_QUESTION, VARIANTS, context, posting_id=posting_id)


_LOAD = """
SELECT s.content_hash, s.factors, s.not_enough_info, s.reason_inputs, j.title, e.data, c.name
FROM scores s
JOIN jobs j ON j.posting_id = s.posting_id
JOIN companies c ON c.id = j.company_id
JOIN extractions e ON e.id = s.extraction_id
WHERE s.posting_id = %s
"""

_STORE = """
INSERT INTO score_reasons (posting_id, content_hash, inputs, prompt_version, model, reason,
    gaps, cv_variant)
VALUES (%s, %s, %s, %s, %s, %s, %s, %s)
ON CONFLICT (posting_id) DO UPDATE SET
    content_hash = excluded.content_hash, inputs = excluded.inputs,
    prompt_version = excluded.prompt_version, model = excluded.model,
    reason = excluded.reason, gaps = excluded.gaps, cv_variant = excluded.cv_variant,
    created_at = now()
"""


def handler(pool: ConnectionPool, reasoner: Reasoner, cfg: Config | None = None) -> Handler:
    """The `reason` work-item handler. The model client's NeedsReview errors fail the item
    at once; nothing is stored then."""
    weights = (cfg or load_config()).weights.model_dump()

    def run(item: Item) -> None:
        with pool.connection() as conn:
            row = conn.execute(_LOAD, (item.posting_id,)).fetchone()
        if row is None:
            raise ItemError("no score for this posting")
        content_hash, factors, nei, inputs, title, data, company = row
        extraction = drop_company(Extraction.model_validate(data), company).model_dump()
        got = reasoner.reason(factors, nei, weights, item.posting_id)
        variant = reasoner.variant(title, extraction, item.posting_id)
        with pool.connection() as conn:
            conn.execute(
                _STORE,
                (
                    item.posting_id,
                    content_hash,
                    inputs,
                    reasoner.prompt.version,
                    MODEL,
                    got.reason,
                    Jsonb(got.gaps[:5]),
                    variant,
                ),
            )

    return run
