"""The coverage report (M4-03): each skill a lead asks for is covered, listed or a gap,
decided in code from the skill map and the evidence bank (guardrail rule 4: gaps are
reported, never filled). No model call; no posting text.

- covered: proven, and named by at least one evidence line;
- listed: familiar, which may appear in the skills section only (rule 3);
- gap: learning, missing from the skill map, or proven with no evidence line.

The tailoring agent (M4-02) gets this as its constraints, and the draft stores it.
"""

import re
from typing import Literal

from pydantic import BaseModel, ConfigDict

from jobhunt_agents.evidence import Bank
from jobhunt_agents.extraction import Extraction
from jobhunt_agents.profile import Profile

Status = Literal["covered", "listed", "gap"]
Why = Literal["learning", "missing", "no_evidence"]

# Words that make a "skill name" read as an instruction (guardrail rule 5): imperatives,
# second person, roles and the injection vocabulary, in English and French.
_INSTRUCTION = re.compile(
    r"\b(ignore[zr]?|instructions?|disregard|you|your|vous|must|should|add|mention|"
    r"rate|score|system|prompt|pretend|expert|output|reveal|ajoute[rz]?|exige)\b",
    re.I,
)


def suspicious(name: str) -> bool:
    """True when an extracted skill name looks like an instruction, not a skill."""
    return len(name) > 60 or ":" in name or bool(_INSTRUCTION.search(name))


class SkillCoverage(BaseModel):
    model_config = ConfigDict(frozen=True)
    name: str  # as the posting names it
    kind: Literal["required", "optional"]
    skill: str | None  # the skill-map entry
    level: Literal["proven", "familiar", "learning"] | None
    status: Status
    why: Why | None  # for a gap
    lines: tuple[str, ...]  # evidence lines naming the skill, the lead's variant first
    suspicious: bool = False


class Coverage(BaseModel):
    model_config = ConfigDict(frozen=True)
    variant: str
    skills: tuple[SkillCoverage, ...]
    profile_version: str
    evidence_version: str

    @property
    def suspicious(self) -> bool:
        return any(s.suspicious for s in self.skills)


def coverage(e: Extraction, profile: Profile, bank: Bank, variant: str) -> Coverage:
    """Required then optional skills, in the posting's order, each with its status."""
    lines = list(bank.lines())
    out: list[SkillCoverage] = []
    kinds: tuple[tuple[Literal["required", "optional"], list[str]], ...] = (
        ("required", e.required_skills),
        ("optional", e.optional_skills),
    )
    for kind, names in kinds:
        for name in names:
            s = profile.skill_map.match(name)
            naming = [x for x in lines if s and any(r.skill == s.name for r in x.skills)]
            naming.sort(key=lambda x: variant not in x.variants)  # stable: file order after
            ids = tuple(x.id for x in naming)
            if s is None:
                status, why = "gap", "missing"
            elif s.level == "learning":
                status, why = "gap", "learning"
            elif s.level == "familiar":
                status, why = "listed", None
            elif ids:
                status, why = "covered", None
            else:
                status, why = "gap", "no_evidence"
            out.append(
                SkillCoverage(
                    name=name,
                    kind=kind,
                    skill=s.name if s else None,
                    level=s.level if s else None,
                    status=status,
                    why=why,
                    lines=ids,
                    suspicious=s is None and suspicious(name),
                )
            )
    return Coverage(
        variant=variant,
        skills=tuple(out),
        profile_version=profile.version,
        evidence_version=bank.version,
    )
