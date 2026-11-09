"""The draft validator (M4-04): every sentence of a CV draft and its letter (M5-01)
checked in code against the evidence lines it cites (evidence-bank-guardrails).
Any violation blocks the draft;
warnings don't. No model, and never bypassed: drafts reach the page only through it.

Violations: unsupported, altered_fact, inflated_number, learning_as_experience,
familiar_in_bullet, too_long (a letter over 250 words, set by tailor.py), and the
claim checker's checker_unsupported and checker_altered_fact (M5-02, checker.py).
Warnings: scope_inflation, suspicious_input.
The independent model claim checker (M5-02) runs on top of this.
"""

import re
from collections.abc import Iterable
from dataclasses import dataclass, field
from typing import Literal

from jobhunt_agents import aliases
from jobhunt_agents.aliases import canon, normalize
from jobhunt_agents.coverage import Coverage
from jobhunt_agents.drafts import Bullet, DraftContent, LetterContent
from jobhunt_agents.evidence import Bank, Line
from jobhunt_agents.numbers import Number, numbers
from jobhunt_agents.profile import Profile

Rule = Literal[
    "unsupported",
    "altered_fact",
    "inflated_number",
    "learning_as_experience",
    "familiar_in_bullet",
    "too_long",
    "checker_unsupported",
    "checker_altered_fact",
    "scope_inflation",
    "suspicious_input",
]


@dataclass(frozen=True)
class Finding:
    rule: Rule
    where: str  # summary[0], roles[ora].bullets[2], skills[3], letter[1][0], draft
    sentence: str
    closest: str | None  # the evidence line it most resembles
    detail: str


@dataclass
class Report:
    violations: list[Finding] = field(default_factory=list[Finding])
    warnings: list[Finding] = field(default_factory=list[Finding])

    @property
    def status(self) -> Literal["blocked", "ready_for_review"]:
        return "blocked" if self.violations else "ready_for_review"


# Titles: up to three capitalised words before a job noun ("Senior Software Engineer").
_TITLE = re.compile(
    r"\b((?:[A-Z][\w+#./-]*\s+){0,3})(engineer|developer|architect|manager|intern|"
    r"consultant|scientist|lead)\b",
    re.I,
)
# French titles put the noun first; a seniority word after it is the inflation to catch
# ("ingénieur logiciel senior"), M7-02. ponytail: seniority words only, not every adjective.
_TITLE_FR = re.compile(
    r"\b(?:ingénieur|ingenieur|développeur|developpeur|architecte|consultant|chef de projet)"
    r"(?:\s+[\w-]+){0,2}?\s+(?:senior|principal|confirmé|expérimenté|lead|staff|junior|en chef)\b",
    re.I,
)
_LEADING = {"as", "a", "an", "the", "our", "my", "former", "previously", "then", "now", "while"}
_YEAR = re.compile(r"\b(19|20)\d{2}\b")
# Ownership and leadership verbs by strength (M4-04 AC7), with French equivalents.
_SCOPE: tuple[tuple[int, re.Pattern[str]], ...] = (
    (1, re.compile(r"\b(own(ed|s|ing)?|responsible for|possédé|responsable de)\b", re.I)),
    (2, re.compile(r"\b(architect(ed|s|ing)?|architecturé|conçu l'architecture)\b", re.I)),
    (
        3,
        re.compile(
            r"\b(lead|leads|leading|led|spearhead(ed|s|ing)?|head(ed|s|ing)?|"
            r"dirigé|piloté|mené)\b",
            re.I,
        ),
    ),
    (4, re.compile(r"\b(manag(e|ed|es|ing)|géré|encadré|managé)\b", re.I)),
)


def _scope(text: str) -> int:
    return max((rank for rank, p in _SCOPE if p.search(text)), default=0)


@dataclass(frozen=True)
class _Skill:
    key: str  # canon name
    display: str
    level: Literal["proven", "familiar", "learning"] | None  # None: not in the skill map


class _Skills:
    """Finds skill names in free text: the skill map, the tech-alias table and the lead's
    own skills. Names of three characters or fewer match case-sensitively ("Go", "C")."""

    def __init__(self, profile: Profile, extra: Iterable[str]) -> None:
        self.profile = profile
        names: dict[str, _Skill] = {}
        for s in profile.skill_map.skills:
            for n in (s.name, *s.aliases):
                names[n] = _Skill(canon(s.name), s.name, s.level)
        for name, al in aliases.TABLE.items():
            for n in (name, *al):
                names.setdefault(n, self.resolve(n))
        for n in extra:
            names.setdefault(n, self.resolve(n))
        self.patterns = [
            (re.compile(rf"(?<![\w+#.-]){re.escape(n)}(?![\w+#-])", 0 if len(n) <= 3 else re.I), s)
            for n, s in sorted(names.items(), key=lambda x: -len(x[0]))
            if n.strip()
        ]

    def resolve(self, name: str) -> _Skill:
        s = self.profile.skill_map.match(name)
        return _Skill(canon(s.name), s.name, s.level) if s else _Skill(canon(name), name, None)

    def find(self, text: str) -> dict[str, _Skill]:
        found: dict[str, _Skill] = {}
        taken: list[tuple[int, int]] = []
        for p, s in self.patterns:  # longest names first; a span is used once
            for m in p.finditer(text):
                if any(a < m.end() and m.start() < b for a, b in taken):
                    continue
                taken.append((m.start(), m.end()))
                found.setdefault(s.key, s)
        return found


def _closest(text: str, lines: Iterable[Line]) -> str | None:
    words = set(normalize(text).split())
    best = max(lines, key=lambda x: len(words & set(normalize(x.text).split())), default=None)
    return best.id if best else None


def _matches(n: Number, cited: list[Number]) -> bool:
    return any(
        n.value == c.value
        and (n.unit == c.unit or not n.unit or not c.unit)
        and (not n.plus or c.plus)
        and (n.approx or not c.approx)
        for c in cited
    )


class _Validator:
    def __init__(
        self, bank: Bank, profile: Profile, retired: frozenset[str], cov: Coverage | None
    ) -> None:
        self.bank = bank
        self.lines = {x.id: x for x in bank.lines()}
        self.retired = retired
        posting = [s.name for s in cov.skills] if cov else []
        self.skills = _Skills(profile, posting)
        self.names = (
            {s.name for s in profile.skill_map.skills}
            | {n for s in profile.skill_map.skills for n in s.aliases}
            | {n for name, al in aliases.TABLE.items() for n in (name, *al)}
        )
        self.parents: dict[str, tuple[str, list[str]]] = {}  # id -> (title or name, years)
        for r in bank.roles:
            self.parents[r.id] = (r.title, [r.start[:4], r.end[:4]])
        for p in bank.projects:
            self.parents[p.id] = (p.name, [(p.start or "")[:4], (p.end or "")[:4]])
        self.orgs = {r.employer: r.id for r in bank.roles} | {p.name: p.id for p in bank.projects}
        self.orgs |= {e.school: e.id for e in bank.education}
        self.report = Report()

    def add(self, rule: Rule, where: str, sentence: str, cited: list[Line], detail: str) -> None:
        f = Finding(rule, where, sentence, _closest(sentence, cited or self.lines.values()), detail)
        (
            self.report.warnings
            if rule in ("scope_inflation", "suspicious_input")
            else self.report.violations
        ).append(f)

    def bullet(self, b: Bullet, where: str, kind: str, section: str) -> None:
        """One sentence: the first violation found, then the scope warning."""
        known = [self.lines[i] for i in b.evidence_ids if i in self.lines and i not in self.retired]
        # the summary and the letter may cite any section; letter lines only the letter
        foreign = [
            x
            for x in known
            if (kind not in ("summary", "letter") and x.parent != section)
            or (kind != "letter" and x.kind == "letter")
        ]
        if not b.evidence_ids or len(known) != len(b.evidence_ids) or foreign:
            detail = "no evidence, an unknown or retired line, or a line from another section"
            self.add("unsupported", where, b.text, known, detail)
            return
        c = _Cited(known, kind, section, self)
        for check in (self._skills_named, self._facts, self._numbers):
            found = check(b.text, c)
            if found:
                self.add(found[0], where, b.text, known, found[1])
                return
        if _scope(b.text) > _scope(c.text):
            detail = "an ownership or leadership verb the cited lines don't use"
            self.add("scope_inflation", where, b.text, known, detail)

    def _skills_named(self, text: str, c: _Cited) -> tuple[Rule, str] | None:
        """A bullet names a skill only if a cited line names or tags it (any level)."""
        for key, s in self.skills.find(text).items():
            if c.kind in ("summary", "letter") and s.level == "learning":
                return "learning_as_experience", f"{s.display} is learning: never in the {c.kind}"
            if key in c.skills:
                continue
            if s.level == "familiar":
                return "familiar_in_bullet", f"{s.display} is familiar and not in the cited lines"
            return "unsupported", f"{s.display} is not in the cited evidence"
        return None

    def _facts(self, text: str, c: _Cited) -> tuple[Rule, str] | None:
        """Titles, years and organisations must come from the evidence."""
        for m in _TITLE.finditer(text):
            words = (m.group(1) + m.group(2)).split()
            while words and words[0].lower() in _LEADING:
                words = words[1:]
            if len(words) > 1 and normalize(" ".join(words)) not in c.allowed:
                return "altered_fact", f"title {' '.join(words)!r} isn't in the evidence"
        for m in _TITLE_FR.finditer(text):
            if normalize(m.group(0)) not in c.allowed:
                return "altered_fact", f"title {m.group(0)!r} isn't in the evidence"
        for m in _YEAR.finditer(text):
            if m.group(0) not in c.years:
                return "altered_fact", f"year {m.group(0)} isn't in the evidence"
        for org, oid in self.orgs.items():
            mentioned = org and normalize(org) in normalize(text)
            if mentioned and oid not in c.parents and normalize(org) not in normalize(c.text):
                return "altered_fact", f"{org!r} isn't this section's"
        return None

    def _numbers(self, text: str, c: _Cited) -> tuple[Rule, str] | None:
        """Every number must be one of the cited lines' (years are facts, above)."""
        cited = [n for x in c.lines for n in x.numbers]
        for n in numbers(text, self.names | set(self.orgs)):
            if n.unit == "" and n.value.is_integer() and 1900 <= n.value <= 2099:
                continue
            if not _matches(n, cited):
                shown = f"{n.value:g}{' ' + n.unit if n.unit else ''}{'+' if n.plus else ''}"
                return "inflated_number", f"{shown} isn't in the cited evidence"
        return None

    def skills_section(self, skills: list[str]) -> None:
        for i, name in enumerate(skills):
            s = self.skills.resolve(name)
            where = f"skills[{i}]"
            if s.level is None:
                self.add("unsupported", where, name, [], f"{name} is not in your skill map")
            elif s.level == "learning":
                self.add("learning_as_experience", where, name, [], f"{name} is learning")


class _Cited:
    """What a sentence's cited lines allow: their text, skills, titles and years."""

    def __init__(self, lines: list[Line], kind: str, section: str, v: _Validator) -> None:
        self.lines = lines
        self.kind = kind
        self.text = " ".join(x.text for x in lines)
        self.parents: set[str] = {x.parent for x in lines}
        if kind not in ("summary", "letter"):
            self.parents.add(section)
        titles = " ".join(v.parents[p][0] for p in self.parents if p in v.parents)
        context = f"{self.text} {titles} {v.bank.person.headline}"
        self.allowed = normalize(context)
        # skills the lines tag or name, and the words of the role's own title ("Backend")
        self.skills = {canon(r.skill) for x in lines for r in x.skills} | set(
            v.skills.find(context)
        )
        self.years = {y for p in self.parents if p in v.parents for y in v.parents[p][1]}
        self.years |= set(re.findall(r"\b(?:19|20)\d{2}\b", self.text))


def validate(
    draft: DraftContent,
    bank: Bank,
    profile: Profile,
    *,
    retired: Iterable[str] = (),
    coverage: Coverage | None = None,
    letter: LetterContent | None = None,
) -> Report:
    """Checks every sentence (CV and letter) against its cited evidence; the draft is
    blocked if any violation is found."""
    v = _Validator(bank, profile, frozenset(retired), coverage)
    for i, b in enumerate(draft.summary):
        v.bullet(b, f"summary[{i}]", "summary", "summary")
    for kind, sections in (("roles", draft.roles), ("projects", draft.projects)):
        for sec in sections:
            for i, b in enumerate(sec.bullets):
                v.bullet(b, f"{kind}[{sec.id}].bullets[{i}]", kind[:-1], sec.id)
    v.skills_section(draft.skills)
    for p, para in enumerate(letter.paragraphs if letter else []):
        for i, b in enumerate(para.sentences):
            v.bullet(b, f"letter[{p}][{i}]", "letter", "letter")
    if coverage is not None and coverage.suspicious:
        names = ", ".join(s.name[:40] for s in coverage.skills if s.suspicious)
        v.add(
            "suspicious_input",
            "draft",
            "",
            [],
            f"the posting's skills look like instructions: {names}",
        )
    return v.report
