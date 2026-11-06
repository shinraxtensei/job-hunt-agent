"""The coded score (M3-06 AC1): every factor computed here from structured inputs (the
extraction, the job's facets, the profile files), weighted by scoring.yaml. The model
never sets or changes it.

Each factor is a value from 0 to 1; the stored points are the weighted sum of all but
freshness, which is added when read (`total`) so a score never goes stale.
"""

import hashlib
import json
import re
from dataclasses import dataclass
from datetime import datetime
from importlib import resources
from pathlib import Path
from typing import Any, Literal, cast

from pydantic import BaseModel, ConfigDict, model_validator
from ruamel.yaml import YAML

from jobhunt_agents import aliases
from jobhunt_agents.aliases import normalize
from jobhunt_agents.extraction import Extraction
from jobhunt_agents.profile import Profile

FitLevel = Literal["very_strong", "strong", "good", "stretch", "no"]
FIT_LEVELS: tuple[FitLevel, ...] = ("very_strong", "strong", "good", "stretch", "no")
# the extraction's levels and the job facets' (intern, staff_plus, lead_manager)
_LADDER = {
    "intern": 0,
    "junior": 0,
    "mid": 1,
    "senior": 2,
    "lead": 3,
    "staff_plus": 3,
    "lead_manager": 3,
}


class _Strict(BaseModel):
    model_config = ConfigDict(frozen=True, extra="forbid")


class Weights(_Strict):
    required: float
    title: float
    seniority: float
    location: float
    optional: float
    freshness: float

    @model_validator(mode="after")
    def _sum(self) -> Weights:
        if abs(sum(self.model_dump().values()) - 100) > 1e-9:
            raise ValueError("weights must sum to 100")
        return self


class Credit(_Strict):
    proven: float
    familiar: float
    learning: float
    missing: float
    none_stated: float


class TitleCfg(_Strict):
    rank_step: float
    none: float


class SeniorityCfg(_Strict):
    by_gap: dict[int, float]
    unknown: float
    per_year_short: float


class LocationCfg(_Strict):
    remote_scopes: tuple[str, ...]
    remote_elsewhere: float
    home_country: str
    relocation_countries: tuple[str, ...]
    relocation: float
    elsewhere: float
    unknown: float
    visa: dict[str, float]


class FreshnessCfg(_Strict):
    fresh_days: float
    stale_days: float
    unknown: float


class Bands(_Strict):
    very_strong: float
    strong: float
    good: float
    stretch: float

    @model_validator(mode="after")
    def _order(self) -> Bands:
        if not self.very_strong > self.strong > self.good > self.stretch:
            raise ValueError("bands must go down: very_strong > strong > good > stretch")
        return self


class Config(_Strict):
    weights: Weights
    credit: Credit
    title: TitleCfg
    seniority: SeniorityCfg
    location: LocationCfg
    freshness: FreshnessCfg
    bands: Bands
    weights_version: str
    bands_version: str


SCORE_CODE = 3  # bump when scoring code changes what a score is: every lead re-scores
# 2: skills named like the posting's company are dropped (M3-06 review)
# 3: freshness in whole days; stored factors carry their weight (M3-08)


def _hash(v: object) -> str:
    return hashlib.sha256(json.dumps(v, sort_keys=True).encode()).hexdigest()


def load_config(path: Path | None = None) -> Config:
    """scoring.yaml (packaged, or `path`), validated; versions hash its sections."""
    text = path.read_text() if path else (resources.files(__package__) / "scoring.yaml").read_text()
    raw = cast(dict[str, Any], YAML(typ="safe").load(text))  # pyright: ignore[reportUnknownMemberType]
    rest = {k: v for k, v in raw.items() if k != "bands"}
    rest |= {"code": SCORE_CODE, "aliases": _hash(aliases.TABLE)}  # matching changes too
    return Config.model_validate(
        raw | {"weights_version": _hash(rest), "bands_version": _hash(raw.get("bands"))}
    )


@dataclass(frozen=True)
class Job:
    """The job facets scoring reads (M2, ADR-0011)."""

    title: str
    seniority: str
    experience_min_years: int | None
    workplace: str
    remote_scopes: tuple[str, ...]
    countries: tuple[str, ...]
    locations: tuple[str, ...]
    published_at: datetime | None


class Match(_Strict):
    name: str  # as the posting names it
    skill: str | None  # the skill-map entry, None when missing
    level: Literal["proven", "familiar", "learning", "missing"]


class Factor(_Strict):
    value: float  # 0..1
    detail: str = ""  # how it was decided, for the reason and the UI
    matches: tuple[Match, ...] = ()  # skill factors only


class Scored(_Strict):
    points: float  # weighted sum without freshness
    factors: dict[str, Factor]
    not_enough_info: bool  # no required skills stated: the reason says so
    weights_version: str
    bands_version: str
    profile_version: str


def skills(names: list[str], profile: Profile, cfg: Config) -> Factor:
    if not names:
        return Factor(value=cfg.credit.none_stated, detail="none stated")
    matches: list[Match] = []
    for n in names:
        s = profile.skill_map.match(n)
        matches.append(
            Match(name=n, skill=s.name, level=s.level)
            if s
            else Match(name=n, skill=None, level="missing")
        )
    credit = cfg.credit.model_dump()
    value = sum(credit[m.level] for m in matches) / len(matches)
    return Factor(value=value, matches=tuple(matches))


def _words(s: str) -> str:
    return " " + re.sub(r"[^\w]+", " ", normalize(s)).strip() + " "


def title(job_title: str, profile: Profile, cfg: Config) -> Factor:
    t = _words(job_title)
    for want in profile.search_profile.titles:  # ranked, best first
        for form in (*want.en, *want.fr):
            if _words(form) in t:
                return Factor(
                    value=max(0.0, 1 - (want.rank - 1) * cfg.title.rank_step),
                    detail=f"target title rank {want.rank}",
                )
    return Factor(value=cfg.title.none, detail="matches none of the target titles")


def seniority(e: Extraction, job: Job, profile: Profile, cfg: Config) -> Factor:
    c = cfg.seniority
    level = e.seniority if e.seniority != "unknown" else job.seniority
    if level in _LADDER:
        gap = _LADDER[level] - _LADDER[profile.search_profile.experience.seniority]
        level_v = c.by_gap[max(-3, min(3, gap))]
    else:
        level_v = c.unknown
    have = profile.search_profile.experience.professional_years
    need = e.min_years if e.min_years is not None else job.experience_min_years
    years_v = 1.0 if need is None else max(0.0, 1 - max(0.0, need - have) * c.per_year_short)
    return Factor(
        value=(level_v + years_v) / 2,
        detail=f"level {level}, minimum {need if need is not None else 'none'} years",
    )


def location(e: Extraction, job: Job, cfg: Config) -> Factor:
    c = cfg.location
    if job.workplace == "remote" or e.remote_policy == "remote":
        if set(job.remote_scopes) & set(c.remote_scopes):
            return Factor(value=1.0, detail="remote, open to Morocco")
        if job.remote_scopes or not job.countries:
            return Factor(value=c.remote_elsewhere, detail="remote, scope may exclude Morocco")
    if c.home_country in job.countries:
        return Factor(value=1.0, detail="in Morocco")
    if not job.countries:
        return Factor(value=c.unknown, detail="location unknown")
    base, where = (
        (c.relocation, "relocation")
        if set(job.countries) & set(c.relocation_countries)
        else (c.elsewhere, "elsewhere")
    )
    v = base + c.visa[e.visa_sponsorship]
    return Factor(value=max(0.0, min(1.0, v)), detail=f"{where}, visa {e.visa_sponsorship}")


def freshness(published_at: datetime | None, now: datetime, cfg: Config) -> float:
    c = cfg.freshness
    if published_at is None:
        return c.unknown
    age = (now - published_at).days  # whole days: stable order within a day (M3-08)
    if age <= c.fresh_days:
        return 1.0
    return max(0.0, (c.stale_days - age) / (c.stale_days - c.fresh_days))


def score(e: Extraction, job: Job, profile: Profile, cfg: Config) -> Scored:
    factors = {
        "required": skills(e.required_skills, profile, cfg),
        "title": title(job.title, profile, cfg),
        "seniority": seniority(e, job, profile, cfg),
        "location": location(e, job, cfg),
        "optional": skills(e.optional_skills, profile, cfg),
    }
    w = cfg.weights.model_dump()
    return Scored(
        points=sum(w[k] * f.value for k, f in factors.items()),
        not_enough_info=not e.required_skills,
        factors=factors,
        weights_version=cfg.weights_version,
        bands_version=cfg.bands_version,
        profile_version=profile.version,
    )


def total(s: Scored, published_at: datetime | None, now: datetime, cfg: Config) -> float:
    """The score shown: stored points plus freshness at `now`."""
    return s.points + cfg.weights.freshness * freshness(published_at, now, cfg)


def band(points: float, b: Bands) -> FitLevel:
    for level, cut in (
        ("very_strong", b.very_strong),
        ("strong", b.strong),
        ("good", b.good),
        ("stretch", b.stretch),
    ):
        if points >= cut:
            return cast(FitLevel, level)
    return "no"


JOB_SQL = """
SELECT title, seniority, experience_min_years, workplace, remote_scopes, countries,
       locations, published_at
FROM jobs WHERE posting_id = %s
"""


def job_from_row(row: tuple[Any, ...]) -> Job:
    """A row of JOB_SQL."""
    t, sen, years, workplace, scopes, countries, locations, published = row
    return Job(
        title=t,
        seniority=sen,
        experience_min_years=years,
        workplace=workplace,
        remote_scopes=tuple(scopes),
        countries=tuple(countries),
        locations=tuple(locations),
        published_at=published,
    )
