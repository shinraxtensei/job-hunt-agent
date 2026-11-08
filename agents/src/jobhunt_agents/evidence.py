# ruamel.yaml is untyped (ADR-0019): values are checked by hand here and turned into
# typed models before leaving this module.
# pyright: reportUnknownMemberType=false, reportUnknownVariableType=false, reportUnknownArgumentType=false
"""The evidence bank (M4-01, ADR-0024): the owner's checked Master CV lines in
data/profile/evidence.yaml, the only source a tailored CV may draw on (guardrail rule 1).

Validated with line numbers before anything uses it; each line records its skills with
their levels in the skill map and the numbers it states (numbers.py). Personal: never
logged beyond ids and counts.
"""

import hashlib
import logging
import re
from collections.abc import Iterator
from pathlib import Path
from typing import Any, Literal, cast

from pydantic import BaseModel, ConfigDict
from ruamel.yaml import YAML
from ruamel.yaml.error import MarkedYAMLError

from jobhunt_agents import aliases
from jobhunt_agents.numbers import Number, numbers
from jobhunt_agents.profile import Level, Profile

log = logging.getLogger(__name__)

VARIANTS = ("BE", "PAY", "DIST", "FS", "AI", "CONS")
_RESERVED = {"person", "summary", "languages"}  # item ids the bank uses itself
_DATE = re.compile(r"^(\d{4})(-(0[1-9]|1[0-2]))?$")
Kind = Literal["summary", "role", "project"]


class _Frozen(BaseModel):
    model_config = ConfigDict(frozen=True)


class Person(_Frozen):
    name: str
    headline: str = ""
    email: str = ""
    phone: str = ""
    location: str = ""
    links: tuple[str, ...] = ()


class SkillRef(_Frozen):
    name: str  # as written in the line
    skill: str  # the skill-map entry
    level: Level


class Line(_Frozen):
    id: str
    kind: Kind
    parent: str  # role or project id; "summary"
    variants: tuple[str, ...]
    skills: tuple[SkillRef, ...]
    text: str
    source: str | None
    numbers: tuple[Number, ...]


class Role(_Frozen):
    id: str
    employer: str
    title: str
    start: str
    end: str
    location: str = ""
    lines: tuple[Line, ...]


class Project(_Frozen):
    id: str
    name: str
    stack: str = ""
    start: str | None = None
    end: str | None = None
    lines: tuple[Line, ...]


class Education(_Frozen):
    id: str
    school: str
    degree: str
    location: str = ""
    start: str
    end: str


class Certification(_Frozen):
    id: str
    name: str
    issuer: str
    date: str


class Language(_Frozen):
    name: str
    level: str


class Bank(_Frozen):
    person: Person
    summary: tuple[Line, ...]
    roles: tuple[Role, ...]
    projects: tuple[Project, ...]
    education: tuple[Education, ...]
    certifications: tuple[Certification, ...]
    languages: tuple[Language, ...]
    version: str  # sha256 of the file and the skill map's version: stored with every draft

    def lines(self) -> Iterator[Line]:
        yield from self.summary
        for r in self.roles:
            yield from r.lines
        for p in self.projects:
            yield from p.lines

    def line(self, line_id: str) -> Line:
        return next(x for x in self.lines() if x.id == line_id)


class EvidenceError(Exception):
    def __init__(self, problems: list[str]) -> None:
        super().__init__("invalid evidence:\n  " + "\n  ".join(problems))
        self.problems = problems


class _Reader:
    """Walks the parsed file, collecting every problem with its line number."""

    def __init__(self, profile: Profile) -> None:
        self.profile = profile
        self.problems: list[tuple[int, str]] = []
        self.ids: dict[str, int] = {}
        self.names: set[str] = set()  # employer, school, project names: not numbers

    def add(self, line: int, msg: str) -> None:
        self.problems.append((line, f"evidence.yaml line {line}: {msg}"))

    @staticmethod
    def at(m: Any, key: str | None = None, default: int = 1) -> int:
        try:
            return int(m.lc.key(key)[0]) + 1 if key is not None else int(m.lc.line) + 1
        except AttributeError, KeyError, TypeError:
            return default

    @staticmethod
    def at_item(seq: Any, i: int) -> int:
        return int(seq.lc.item(i)[0]) + 1

    def keys(self, m: Any, allowed: tuple[str, ...], required: tuple[str, ...]) -> bool:
        ok = True
        for k in m:
            if k not in allowed:
                self.add(self.at(m, k), f"unknown key {k!r}")
                ok = False
        for k in required:
            if k not in m or m[k] in (None, ""):
                self.add(self.at(m), f"{k}: required")
                ok = False
        return ok

    def text(self, m: Any, key: str) -> str:
        v = m.get(key)
        if v is None:
            return ""
        if not isinstance(v, str | int):
            self.add(self.at(m, key), f"{key}: want text")
            return ""
        return str(v).strip()

    def unique(self, m: Any, item_id: str) -> None:
        line = self.at(m, "id")
        if item_id in _RESERVED:
            self.add(line, f"id {item_id!r} is reserved; pick another")
        elif item_id in self.ids:
            self.add(line, f"id {item_id!r} is also used on line {self.ids[item_id]}")
        else:
            self.ids[item_id] = line

    def date(self, m: Any, key: str, *, required: bool = True) -> str | None:
        if key not in m or m[key] is None:
            if required:
                self.add(self.at(m), f"{key}: required")
            return None
        v = str(m[key]).strip()
        if v != "present" and not _DATE.match(v):
            self.add(self.at(m, key), f"{key} {v!r}: want YYYY-MM, YYYY or present")
        return v

    def lines(self, seq: Any, kind: Kind, parent: str) -> list[Line]:
        out: list[Line] = []
        if not isinstance(seq, list):
            return out
        for i, raw in enumerate(cast(list[Any], seq)):
            if not isinstance(raw, dict):
                self.add(self.at_item(seq, i), "want a line: {id, variants, skills, text}")
                continue
            m = cast(Any, raw)
            if not self.keys(
                m, ("id", "variants", "skills", "text", "source", "checked"), ("id", "text")
            ):
                continue
            line_id = self.text(m, "id")
            self.unique(m, line_id)
            variants = [str(v) for v in m.get("variants") or []]
            for v in variants:
                if v not in VARIANTS:
                    self.add(self.at(m, "variants"), f"variant {v!r}: want {' '.join(VARIANTS)}")
            refs: list[SkillRef] = []
            for name in [str(s) for s in m.get("skills") or []]:
                s = self.profile.skill_map.match(name)
                if s is None:
                    self.add(self.at(m, "skills"), f"skill {name!r} is not in your skill map")
                    continue
                if s.level == "learning" and kind != "project":
                    self.add(
                        self.at(m, "skills"),
                        f"learning skill {s.name!r} is allowed only in a project line",
                    )
                refs.append(SkillRef(name=name, skill=s.name, level=s.level))
            if m.get("checked") is not True:
                self.add(self.at(m, "checked") if "checked" in m else self.at(m), "not checked")
            text = self.text(m, "text")
            out.append(
                Line(
                    id=line_id,
                    kind=kind,
                    parent=parent,
                    variants=tuple(variants),
                    skills=tuple(refs),
                    text=text,
                    source=self.text(m, "source") or None,
                    numbers=(),  # filled once every name in the file is known
                )
            )
        return out


def _items(doc: Any, key: str) -> list[Any]:
    v = doc.get(key)
    return cast(list[Any], v) if isinstance(v, list) else []


def _parse(raw: bytes, profile: Profile) -> Bank:
    try:
        doc = YAML(typ="rt").load(raw.decode())
    except MarkedYAMLError as e:
        mark = e.problem_mark or e.context_mark
        raise EvidenceError(
            [f"evidence.yaml line {mark.line + 1 if mark else 1}: invalid YAML: {e.problem}"]
        ) from None
    if not isinstance(doc, dict):
        raise EvidenceError(["evidence.yaml line 1: want a mapping with person, roles, ..."])
    r = _Reader(profile)
    d = cast(Any, doc)
    top = ("person", "summary", "roles", "projects", "education", "certifications", "languages")
    r.keys(d, top, ("person",))
    pm = d.get("person") or {}
    r.keys(pm, ("name", "headline", "email", "phone", "location", "links"), ("name",))
    person = Person(
        name=r.text(pm, "name"),
        headline=r.text(pm, "headline"),
        email=r.text(pm, "email"),
        phone=r.text(pm, "phone"),
        location=r.text(pm, "location"),
        links=tuple(str(x) for x in pm.get("links") or []),
    )
    summary = r.lines(d.get("summary"), "summary", "summary")
    roles: list[Role] = []
    for m in _items(d, "roles"):
        role_keys = ("id", "employer", "title", "start", "end", "location", "lines")
        if not r.keys(m, role_keys, ("id", "employer", "title")):
            continue
        rid = r.text(m, "id")
        r.unique(m, rid)
        r.names.add(r.text(m, "employer"))
        roles.append(
            Role(
                id=rid,
                employer=r.text(m, "employer"),
                title=r.text(m, "title"),
                start=r.date(m, "start") or "",
                end=r.date(m, "end") or "",
                location=r.text(m, "location"),
                lines=tuple(r.lines(m.get("lines"), "role", rid)),
            )
        )
    projects: list[Project] = []
    for m in _items(d, "projects"):
        if not r.keys(m, ("id", "name", "stack", "start", "end", "lines"), ("id", "name")):
            continue
        pid = r.text(m, "id")
        r.unique(m, pid)
        r.names.add(r.text(m, "name"))
        projects.append(
            Project(
                id=pid,
                name=r.text(m, "name"),
                stack=r.text(m, "stack"),
                start=r.date(m, "start", required=False),
                end=r.date(m, "end", required=False),
                lines=tuple(r.lines(m.get("lines"), "project", pid)),
            )
        )
    education: list[Education] = []
    for m in _items(d, "education"):
        edu_keys = ("id", "school", "degree", "location", "start", "end")
        if not r.keys(m, edu_keys, ("id", "school", "degree")):
            continue
        eid = r.text(m, "id")
        r.unique(m, eid)
        r.names.add(r.text(m, "school"))
        education.append(
            Education(
                id=eid,
                school=r.text(m, "school"),
                degree=r.text(m, "degree"),
                location=r.text(m, "location"),
                start=r.date(m, "start") or "",
                end=r.date(m, "end") or "",
            )
        )
    certs: list[Certification] = []
    for m in _items(d, "certifications"):
        if not r.keys(m, ("id", "name", "issuer", "date"), ("id", "name", "issuer")):
            continue
        cid = r.text(m, "id")
        r.unique(m, cid)
        certs.append(
            Certification(
                id=cid,
                name=r.text(m, "name"),
                issuer=r.text(m, "issuer"),
                date=r.date(m, "date") or "",
            )
        )
    languages = [
        Language(name=r.text(m, "name"), level=r.text(m, "level"))
        for m in _items(d, "languages")
        if r.keys(m, ("name", "level"), ("name", "level"))
    ]
    if r.problems:
        raise EvidenceError([msg for _, msg in sorted(r.problems)])
    names = _known_names(profile) | r.names

    def counted(ls: list[Line] | tuple[Line, ...]) -> tuple[Line, ...]:
        return tuple(x.model_copy(update={"numbers": tuple(numbers(x.text, names))}) for x in ls)

    return Bank(
        person=person,
        summary=counted(summary),
        roles=tuple(x.model_copy(update={"lines": counted(x.lines)}) for x in roles),
        projects=tuple(x.model_copy(update={"lines": counted(x.lines)}) for x in projects),
        education=tuple(education),
        certifications=tuple(certs),
        languages=tuple(languages),
        version=_version(raw, profile),
    )


def _known_names(profile: Profile) -> set[str]:
    """Skill and product names: a number right after one is its version ("Python 3")."""
    out = {n for s in profile.skill_map.skills for n in (s.name, *s.aliases)}
    out |= {n for name, al in aliases.TABLE.items() for n in (name, *al)}
    return out


def _version(raw: bytes, profile: Profile) -> str:
    return hashlib.sha256(raw + b"\0" + profile.version.encode()).hexdigest()


def load_evidence(path: Path, profile: Profile) -> Bank:
    """Loads and validates the file; EvidenceError lists every problem."""
    return _parse(path.read_bytes(), profile)


class EvidenceWatcher:
    """The current bank, reloaded when the file or the skill map changes. A broken edit is
    logged and the last good version kept; no file means no bank (M4 waits for OI-1)."""

    def __init__(self, path: Path) -> None:
        self._path = path
        self._bank: Bank | None = None
        self._warned = False
        self._failed: str | None = None  # the broken version already reported

    def current(self, profile: Profile) -> Bank | None:
        if not self._path.is_file():
            if not self._warned:
                log.info("no evidence.yaml in PROFILE_DIR; the evidence bank stays empty")
                self._warned = True
            return self._bank
        raw = self._path.read_bytes()
        version = _version(raw, profile)
        if version in (self._failed, self._bank.version if self._bank else None):
            return self._bank
        try:
            self._bank = _parse(raw, profile)
            log.info("evidence loaded", extra={"version": self._bank.version[:12]})
        except EvidenceError as e:
            self._failed = version
            log.warning(
                "evidence not loaded; keeping the last good version",
                extra={"error": f"{len(e.problems)} problems, first: {e.problems[0][:120]}"},
            )
        return self._bank
