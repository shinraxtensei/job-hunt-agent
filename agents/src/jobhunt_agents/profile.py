# ruamel.yaml is untyped (ADR-0019): its values are checked by hand here and turned into
# typed Pydantic models before leaving this module.
# pyright: reportUnknownMemberType=false, reportUnknownVariableType=false, reportUnknownArgumentType=false
"""The owner's skill map and search profile (M3-05, ADR-0019).

Two personal, git-ignored YAML files in PROFILE_DIR, validated with line numbers before
anything uses them. Skill names from postings resolve through names and aliases,
ignoring case, accents and punctuation, and through the committed tech-alias table
(aliases.py). The content hash is the profile's version.
"""

import hashlib
import logging
from pathlib import Path
from typing import Any, Literal, cast

from pydantic import BaseModel, ConfigDict
from ruamel.yaml import YAML
from ruamel.yaml.error import MarkedYAMLError

from jobhunt_agents.aliases import canon

log = logging.getLogger(__name__)

Level = Literal["proven", "familiar", "learning"]
LEVELS: tuple[Level, ...] = ("proven", "familiar", "learning")
Seniority = Literal["junior", "mid", "senior"]
SENIORITIES: tuple[Seniority, ...] = ("junior", "mid", "senior")
FILES = ("skill_map.yaml", "search_profile.yaml")


class _Frozen(BaseModel):
    model_config = ConfigDict(frozen=True)


class Skill(_Frozen):
    name: str
    level: Level
    aliases: tuple[str, ...] = ()
    evidence: str | None = None  # where the work is shown; required when proven


class Title(_Frozen):
    rank: int  # 1 = most wanted
    en: tuple[str, ...]
    fr: tuple[str, ...]


class Experience(_Frozen):
    professional_years: float
    seniority: Seniority


class SkillMap(_Frozen):
    skills: tuple[Skill, ...]
    by_key: dict[str, Skill]  # normalized name or alias -> skill

    def match(self, name: str) -> Skill | None:
        """The skill a name from a posting refers to, or None."""
        return self.by_key.get(canon(name))


class SearchProfile(_Frozen):
    titles: tuple[Title, ...]
    experience: Experience


class Profile(_Frozen):
    skill_map: SkillMap
    search_profile: SearchProfile
    version: str  # sha256 of both files: stored with every score (M3-06)


class ProfileError(Exception):
    def __init__(self, problems: list[str]) -> None:
        super().__init__("invalid profile:\n  " + "\n  ".join(problems))
        self.problems = problems


class _Problems:
    def __init__(self, file: str) -> None:
        self.file = file
        self.items: list[tuple[int, str]] = []

    def add(self, line: int, msg: str) -> None:
        self.items.append((line, f"{self.file} line {line}: {msg}"))


def _line_of_key(m: Any, key: str) -> int:
    return int(m.lc.key(key)[0]) + 1


def _line_of_item(seq: Any, i: int) -> int:
    return int(seq.lc.item(i)[0]) + 1


def _texts(v: Any) -> list[str] | None:
    if isinstance(v, list) and all(isinstance(x, str) and x.strip() for x in cast(list[Any], v)):
        return [str(x).strip() for x in cast(list[Any], v)]
    return None


def _parse(file: str, text: str, problems: _Problems) -> Any:
    try:
        return YAML(typ="rt").load(text)
    except MarkedYAMLError as e:
        mark = e.problem_mark or e.context_mark
        problems.add(mark.line + 1 if mark else 1, f"invalid YAML: {e.problem or e.context}")
        return None


def _skill_map(text: str, problems: _Problems) -> SkillMap | None:
    doc = _parse(problems.file, text, problems)
    if problems.items:
        return None
    if not isinstance(doc, dict) or "skills" not in doc:
        problems.add(1, "want a mapping with skills: [...]")
        return None
    seq = cast(Any, doc)["skills"]
    if not isinstance(seq, list):
        problems.add(_line_of_key(doc, "skills"), "skills: want a list")
        return None
    skills: list[Skill] = []
    owners: dict[str, tuple[str, int]] = {}  # normalized key -> (skill name, line)
    for i, raw in enumerate(cast(list[Any], seq)):
        line = _line_of_item(seq, i)
        if not isinstance(raw, dict):
            problems.add(line, "want a skill: {name, level, aliases, evidence}")
            continue
        m = cast(Any, raw)
        for k in m:
            if k not in ("name", "level", "aliases", "evidence"):
                problems.add(_line_of_key(m, k), f"unknown key {k!r}")
        name = m.get("name")
        if not isinstance(name, str) or not name.strip():
            problems.add(_line_of_key(m, "name") if "name" in m else line, "name: required text")
            continue
        level = m.get("level")
        if level not in LEVELS:
            where = _line_of_key(m, "level") if "level" in m else line
            problems.add(where, f"level {level!r}: want proven, familiar or learning")
            continue
        aliases: list[str] = []
        if "aliases" in m:
            got = _texts(m["aliases"])
            if got is None:
                problems.add(_line_of_key(m, "aliases"), "aliases: want a list of names")
            else:
                aliases = got
        evidence = m.get("evidence")
        if evidence is not None and (not isinstance(evidence, str) or not evidence.strip()):
            problems.add(_line_of_key(m, "evidence"), "evidence: want text")
            evidence = None
        if level == "proven" and not evidence:
            problems.add(line, f"proven skill {name!r} needs evidence (where the work is)")
        for key, kline in [(name, line)] + [(a, _line_of_key(m, "aliases")) for a in aliases]:
            n = canon(key)  # two names for one tech (tech_aliases.yaml) are one skill
            if n in owners and owners[n][0] != name:
                other, oline = owners[n]
                what = "name" if key == name else "alias"
                problems.add(kline, f"{what} {key!r} is also {other!r} (line {oline})")
            owners.setdefault(n, (name, line))
        skills.append(
            Skill(
                name=name.strip(),
                level=level,
                aliases=tuple(aliases),
                evidence=evidence.strip() if isinstance(evidence, str) else None,
            )
        )
    by_key = {n: s for s in skills for n in map(canon, (s.name, *s.aliases))}
    return SkillMap(skills=tuple(skills), by_key=by_key)


def _search_profile(text: str, problems: _Problems) -> SearchProfile | None:
    doc = _parse(problems.file, text, problems)
    if problems.items:
        return None
    if not isinstance(doc, dict) or "titles" not in doc or "experience" not in doc:
        problems.add(1, "want a mapping with titles: [...] and experience: {...}")
        return None
    d = cast(Any, doc)
    for k in d:
        if k not in ("titles", "experience"):
            problems.add(_line_of_key(d, k), f"unknown key {k!r}")
    titles: list[Title] = []
    seq = d["titles"]
    if not isinstance(seq, list) or not seq:
        problems.add(_line_of_key(d, "titles"), "titles: want a non-empty list")
    else:
        for i, raw in enumerate(cast(list[Any], seq)):
            line = _line_of_item(seq, i)
            if not isinstance(raw, dict):
                problems.add(line, "want a title: {en: [...], fr: [...]}")
                continue
            m = cast(Any, raw)
            forms: dict[str, list[str]] = {}
            for k in m:
                if k not in ("en", "fr"):
                    problems.add(_line_of_key(m, k), f"unknown key {k!r}")
            for lang in ("en", "fr"):
                got = _texts(m.get(lang))
                if not got:
                    where = _line_of_key(m, lang) if lang in m else line
                    problems.add(where, f"{lang}: want a non-empty list of titles")
                else:
                    forms[lang] = got
            if len(forms) == 2:
                titles.append(Title(rank=i + 1, en=tuple(forms["en"]), fr=tuple(forms["fr"])))
    exp = d["experience"]
    if not isinstance(exp, dict):
        problems.add(_line_of_key(d, "experience"), "experience: want a mapping")
        return None
    e = cast(Any, exp)
    years, seniority = e.get("professional_years"), e.get("seniority")
    ok = True
    if isinstance(years, bool) or not isinstance(years, int | float) or not 0 <= years <= 60:
        where = _line_of_key(e, "professional_years") if "professional_years" in e else 1
        problems.add(where, f"professional_years {years!r}: want a number from 0 to 60")
        ok = False
    if seniority not in SENIORITIES:
        where = _line_of_key(e, "seniority") if "seniority" in e else 1
        problems.add(where, f"seniority {seniority!r}: want junior, mid or senior")
        ok = False
    if not ok:
        return None
    return SearchProfile(
        titles=tuple(titles),
        experience=Experience(
            professional_years=float(cast(float, years)), seniority=cast(Seniority, seniority)
        ),
    )


def _read(directory: Path) -> dict[str, bytes]:
    out: dict[str, bytes] = {}
    missing = [f for f in FILES if not (directory / f).is_file()]
    if missing:
        raise ProfileError([f"{f}: not found in {directory}" for f in missing])
    for f in FILES:
        out[f] = (directory / f).read_bytes()
    return out


def _version(raw: dict[str, bytes]) -> str:
    h = hashlib.sha256()
    for f in FILES:
        h.update(f.encode() + b"\0" + raw[f] + b"\0")
    return h.hexdigest()


def _build(raw: dict[str, bytes]) -> Profile:
    sm_p, sp_p = _Problems(FILES[0]), _Problems(FILES[1])
    sm = _skill_map(raw[FILES[0]].decode(), sm_p)
    sp = _search_profile(raw[FILES[1]].decode(), sp_p)
    problems = [m for _, m in sorted(sm_p.items)] + [m for _, m in sorted(sp_p.items)]
    if problems or sm is None or sp is None:
        raise ProfileError(problems)
    return Profile(skill_map=sm, search_profile=sp, version=_version(raw))


def load(directory: Path) -> Profile:
    """Loads and validates both files; ProfileError lists every problem."""
    return _build(_read(directory))


class ProfileWatcher:
    """The current profile, reloaded when the files change (no restart). A broken edit
    is logged and the last good version kept."""

    def __init__(self, directory: Path) -> None:
        self._dir = directory
        self._profile = load(directory)

    def current(self) -> Profile:
        try:
            raw = _read(self._dir)
            if _version(raw) == self._profile.version:
                return self._profile
            self._profile = _build(raw)
            log.info("profile reloaded", extra={"version": self._profile.version[:12]})
        except ProfileError as e:
            log.warning(
                "profile not reloaded; keeping the last good version", extra={"error": str(e)}
            )
        return self._profile
