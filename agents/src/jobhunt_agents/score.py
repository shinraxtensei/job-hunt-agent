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

# M9-03: four levels; "stretch" is a flag on the score (Scored.stretch), not a level.
FitLevel = Literal["very_strong", "strong", "good", "no"]
FIT_LEVELS: tuple[FitLevel, ...] = ("very_strong", "strong", "good", "no")
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
    location_gate: float  # location points below this cap the level at good (M3-09, M9-03)

    @model_validator(mode="after")
    def _order(self) -> Bands:
        if not self.very_strong > self.strong > self.good > 0:
            raise ValueError("bands must go down: very_strong > strong > good > 0")
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


SCORE_CODE = 9  # bump when scoring code changes what a score is: every lead re-scores
# 2: skills named like the posting's company are dropped (M3-06 review)
# 3: freshness in whole days; stored factors carry their weight (M3-08)
# 4: the location detail names the place (M3-09)
# 5: ... the matched location first (ADR-0023)
# 6, 7: the CV variant is chosen with the score (M4-06)
# 8: the place is the matched member's (ADR-0030)
# 9: four fit levels, stretch a flag (M9-03)


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
    matched_location: str = ""  # passed the default preset's rules (reconcile, ADR-0023)
    role_family: str = "unknown"  # the job's facet (M2), for the CV variant (M4-06)
    track: str = ""  # the company's track (ADR-0005)


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
    capped: bool  # location/remote/visa below the gate: at most good (M3-09, M9-03)
    stretch: bool  # the job asks for a higher level or more years than the profile (M9-03)
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


def _level(e: Extraction, job: Job) -> str:
    return e.seniority if e.seniority != "unknown" else job.seniority


def _need(e: Extraction, job: Job) -> int | None:
    return e.min_years if e.min_years is not None else job.experience_min_years


def stretch(e: Extraction, job: Job, profile: Profile) -> bool:
    """M9-03: the job is a step up: a higher level than the profile's, or more years than
    it has. Unknown level and no stated years: not a stretch."""
    exp = profile.search_profile.experience
    level, need = _level(e, job), _need(e, job)
    above = level in _LADDER and _LADDER[level] > _LADDER[exp.seniority]
    return above or (need is not None and need > exp.professional_years)


def seniority(e: Extraction, job: Job, profile: Profile, cfg: Config) -> Factor:
    c = cfg.seniority
    level = _level(e, job)
    if level in _LADDER:
        gap = _LADDER[level] - _LADDER[profile.search_profile.experience.seniority]
        level_v = c.by_gap[max(-3, min(3, gap))]
    else:
        level_v = c.unknown
    have = profile.search_profile.experience.professional_years
    need = _need(e, job)
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
    wp = _WORKPLACE.get(job.workplace, "")
    places = [
        p
        for p in (job.matched_location, *job.locations)  # the one that passed, else the first
        if p and normalize(p) not in _WORKPLACE_WORDS
    ]
    at = f"{wp} in {places[0]}" if places else wp or "place not given"
    return Factor(
        value=max(0.0, min(1.0, v)),
        detail=f"{where}: {at.strip()}, visa {e.visa_sponsorship}",  # M3-09: names the place
    )


_WORKPLACE = {"onsite": "on-site", "hybrid": "hybrid", "remote": "remote"}
_WORKPLACE_WORDS = {"hybrid", "remote", "on site", "onsite", "in person"}  # not places


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
        capped=capped(w["location"] * factors["location"].value, cfg.bands),
        stretch=stretch(e, job, profile),
        factors=factors,
        weights_version=cfg.weights_version,
        bands_version=cfg.bands_version,
        profile_version=profile.version,
    )


def total(s: Scored, published_at: datetime | None, now: datetime, cfg: Config) -> float:
    """The score shown: stored points plus freshness at `now`."""
    return s.points + cfg.weights.freshness * freshness(published_at, now, cfg)


def capped(location_points: float, b: Bands) -> bool:
    """M3-09: a lead you can't work at (location/remote/visa below the gate) is at most
    good (M9-03), whatever its other factors."""
    return location_points < b.location_gate


def fit_level(total_points: float, location_points: float, b: Bands) -> FitLevel:
    """The band of the total, capped at good by the location gate. The total itself is
    never changed."""
    level = band(total_points, b)
    if capped(location_points, b) and level in ("very_strong", "strong"):
        return "good"
    return level


def band(points: float, b: Bands) -> FitLevel:
    for level, cut in (
        ("very_strong", b.very_strong),
        ("strong", b.strong),
        ("good", b.good),
    ):
        if points >= cut:
            return cast(FitLevel, level)
    return "no"


# The place comes from the group member that passed the default preset (m, ADR-0030),
# else the job's own. rescore._STALE reads it the same way.
JOB_SQL = """
SELECT j.title, j.seniority, j.experience_min_years,
       coalesce(m.workplace, j.workplace), coalesce(m.remote_scopes, j.remote_scopes),
       coalesce(m.countries, j.countries), coalesce(m.locations, j.locations),
       j.published_at, j.matched_location, j.role_family, coalesce(c.track, '')
FROM jobs j JOIN companies c ON c.id = j.company_id
LEFT JOIN jobs m ON m.posting_id = j.matched_posting_id
WHERE j.posting_id = %s
"""


def job_from_row(row: tuple[Any, ...]) -> Job:
    """A row of JOB_SQL."""
    t, sen, years, workplace, scopes, countries, locations, published, matched, family, track = row
    return Job(
        title=t,
        seniority=sen,
        experience_min_years=years,
        workplace=workplace,
        remote_scopes=tuple(scopes),
        countries=tuple(countries),
        locations=tuple(locations),
        published_at=published,
        matched_location=matched,
        role_family=family,
        track=track,
    )
