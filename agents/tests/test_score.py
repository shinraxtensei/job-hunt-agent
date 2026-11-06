"""M3-06 AC1: every factor is computed in code from structured inputs; the score is the
weighted sum with weights from configuration, the fit level comes from configured bands.
Profile: the committed examples (proven PostgreSQL, Node.js, TypeScript; familiar Docker,
Kubernetes, CI/CD, C++; learning Go; titles Backend Engineer, then Software Engineer;
3 years, mid)."""

import shutil
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest

from jobhunt_agents import aliases
from jobhunt_agents import score as score_module
from jobhunt_agents.evals.fit import Agreement, fit_bands
from jobhunt_agents.extraction import Extraction, drop_company
from jobhunt_agents.profile import Profile, load
from jobhunt_agents.score import (
    Config,
    FitLevel,
    Job,
    band,
    fit_level,
    freshness,
    load_config,
    location,
    seniority,
    skills,
    title,
    total,
)
from jobhunt_agents.score import score as score_lead
from tests.conftest import ROOT

NOW = datetime(2026, 11, 5, tzinfo=UTC)


@pytest.fixture(scope="module")
def profile(tmp_path_factory: pytest.TempPathFactory) -> Profile:
    d = tmp_path_factory.mktemp("profile")
    for name in ("skill_map", "search_profile"):
        shutil.copy(ROOT / "data" / "profile" / f"{name}.example.yaml", d / f"{name}.yaml")
    return load(d)


@pytest.fixture(scope="module")
def cfg() -> Config:
    return load_config()


def ext(**kw: object) -> Extraction:
    base: dict[str, object] = {
        "required_skills": [],
        "optional_skills": [],
        "min_years": None,
        "seniority": "unknown",
        "visa_sponsorship": "unknown",
        "salary": None,
        "languages": [],
        "remote_policy": "unknown",
    }
    return Extraction.model_validate(base | kw)


def job(**kw: object) -> Job:
    base: dict[str, object] = {
        "title": "Backend Engineer",
        "seniority": "unknown",
        "experience_min_years": None,
        "workplace": "unknown",
        "remote_scopes": (),
        "countries": (),
        "locations": (),
        "published_at": NOW,
        "matched_location": "",
    }
    return Job(**base | kw)  # pyright: ignore[reportArgumentType]


@pytest.mark.parametrize(
    ("names", "want"),
    [
        (["PostgreSQL", "TypeScript"], 1.0),  # proven 1.0 each
        (["Postgres", "K8s"], 0.75),  # aliases resolve; proven 1.0 + familiar 0.5
        (["Golang", "Rust"], 0.1),  # learning 0.2 + missing 0
        (["COBOL"], 0.0),
        ([], 0.0),  # nothing stated: no credit (the score is flagged instead)
    ],
)
def test_skills_credit_by_level(
    profile: Profile, cfg: Config, names: list[str], want: float
) -> None:
    assert skills(names, profile, cfg).value == pytest.approx(want)


def test_skills_lists_each_match(profile: Profile, cfg: Config) -> None:
    got = skills(["postgres", "Rust", "Go"], profile, cfg)
    assert [(m.name, m.skill, m.level) for m in got.matches] == [
        ("postgres", "PostgreSQL", "proven"),
        ("Rust", None, "missing"),
        ("Go", "Go", "learning"),
    ]


@pytest.mark.parametrize(
    ("t", "want"),
    [
        ("Backend Engineer", 1.0),
        ("Senior Backend Engineer (Payments) - Berlin", 1.0),  # contains a rank-1 form
        ("Ingénieur Backend H/F", 1.0),  # French form, accents folded
        ("Software Engineer, Platform", 0.9),  # rank 2
        ("Senior Software Engineers Lead", 0.0),  # whole words only
        ("Data Analyst", 0.0),
    ],
)
def test_title_by_rank(profile: Profile, cfg: Config, t: str, want: float) -> None:
    assert title(t, profile, cfg).value == pytest.approx(want)


@pytest.mark.parametrize(
    ("e", "j", "want"),
    [
        (ext(seniority="mid"), job(), 1.0),  # same level, no minimum
        (ext(seniority="senior"), job(), 0.8),  # one up: 0.6, years 1.0
        (ext(), job(seniority="senior"), 0.8),  # the facet when extraction says unknown
        (ext(seniority="junior", min_years=2), job(), 0.9),  # one down 0.8, years ok
        (ext(seniority="lead"), job(), 0.6),  # two up 0.2
        (ext(min_years=5), job(), 0.65),  # unknown 0.8; 2 years short: 1 - 2*0.25
        (ext(), job(experience_min_years=4), 0.775),  # the facet minimum; 1 short
        (ext(min_years=10), job(), 0.4),  # years floor at 0
    ],
)
def test_seniority_and_years(
    profile: Profile, cfg: Config, e: Extraction, j: Job, want: float
) -> None:
    assert seniority(e, j, profile, cfg).value == pytest.approx(want)


@pytest.mark.parametrize(
    ("e", "j", "want"),
    [
        (ext(), job(workplace="remote", remote_scopes=("worldwide",)), 1.0),
        (ext(remote_policy="remote"), job(remote_scopes=("emea", "americas")), 1.0),
        (ext(), job(workplace="remote", remote_scopes=("europe",)), 0.5),  # may exclude MA
        (ext(), job(workplace="onsite", countries=("MA",), locations=("Casablanca",)), 1.0),
        (ext(), job(workplace="hybrid", countries=("DE",)), 0.6),  # relocation country
        (ext(visa_sponsorship="yes"), job(countries=("NL",)), 0.9),
        (ext(visa_sponsorship="no"), job(countries=("FR",)), 0.3),
        (ext(), job(countries=("US",)), 0.2),  # elsewhere
        (ext(), job(), 0.5),  # nothing known
    ],
)
def test_location_remote_visa(cfg: Config, e: Extraction, j: Job, want: float) -> None:
    assert location(e, j, cfg).value == pytest.approx(want)


@pytest.mark.parametrize(
    ("age_days", "want"),
    # whole days (M3-08): the order is stable within a day, for the API's cursor
    [(0, 1.0), (7, 1.0), (7.9, 1.0), (26, 0.5), (26.9, 0.5), (45, 0.0), (400, 0.0), (None, 0.5)],
)
def test_freshness_computed_when_read(cfg: Config, age_days: float | None, want: float) -> None:
    published = None if age_days is None else NOW - timedelta(days=age_days)
    assert freshness(published, NOW, cfg) == pytest.approx(want)


@pytest.mark.parametrize(
    ("points", "want"),
    [(100, "very_strong"), (0, "no")],
)
def test_band_ends(cfg: Config, points: float, want: str) -> None:
    assert band(points, cfg.bands) == want


def test_bands_are_cut_offs(cfg: Config) -> None:
    b = cfg.bands
    assert b.very_strong > b.strong > b.good > b.stretch > 0
    assert band(b.strong, b) == "strong"
    assert band(b.strong - 0.01, b) == "good"


# The score is the weighted sum of the factor values; freshness is added when read.
def test_score_is_the_weighted_sum(profile: Profile, cfg: Config) -> None:
    e = ext(
        required_skills=["PostgreSQL", "Kubernetes"],  # 0.75
        optional_skills=["Rust"],  # 0
        seniority="mid",
        remote_policy="remote",
    )
    s = score_lead(e, job(title="Software Engineer", remote_scopes=("worldwide",)), profile, cfg)
    w = cfg.weights
    want = w.required * 0.75 + w.title * 0.9 + w.seniority * 1.0 + w.location * 1.0
    assert s.points == pytest.approx(want)  # optional 0, freshness not stored
    assert s.factors["optional"].value == 0
    assert total(s, NOW - timedelta(days=60), NOW, cfg) == pytest.approx(want)
    assert total(s, NOW, NOW, cfg) == pytest.approx(want + w.freshness)
    assert s.weights_version == cfg.weights_version
    assert s.profile_version == profile.version


def test_weights_must_sum_to_100(tmp_path: Path) -> None:
    text = (ROOT / "agents/src/jobhunt_agents/scoring.yaml").read_text()
    bad = tmp_path / "scoring.yaml"
    bad.write_text(text.replace("required: 35", "required: 36"))
    with pytest.raises(ValueError, match="sum to 100"):
        load_config(bad)


def test_versions_change_with_their_section(tmp_path: Path, cfg: Config) -> None:
    text = (ROOT / "agents/src/jobhunt_agents/scoring.yaml").read_text()
    f = tmp_path / "scoring.yaml"
    f.write_text(text.replace("familiar: 0.5", "familiar: 0.6"))
    changed = load_config(f)
    assert changed.weights_version != cfg.weights_version
    assert changed.bands_version == cfg.bands_version


# M3-06 band calibration: the fitter finds cut-offs that separate cleanly separable labels,
# and the agreement counts exact and within-one levels.
def test_fit_bands_and_agreement() -> None:
    points = [90, 85, 70, 60, 50, 45, 30, 10]
    labels: list[FitLevel] = [
        "very_strong",
        "very_strong",
        "strong",
        "strong",
        "good",
        "good",
        "stretch",
        "no",
    ]
    b = fit_bands(points, labels)
    a = Agreement(points, labels, b)
    assert (a.exact, a.within_one) == (1.0, 1.0)
    shifted = Agreement(points, ["strong", *labels[1:]], b)
    assert shifted.exact == 7 / 8
    assert shifted.within_one == 1.0
    assert shifted.matrix[1][0] == 1  # owner strong, band very_strong


# An empty required-skill list earns nothing and flags the score as not enough info, so
# the reason says so; a listed one does not.
def test_no_required_skills_flags_not_enough_info(profile: Profile, cfg: Config) -> None:
    empty = score_lead(ext(), job(), profile, cfg)
    assert empty.factors["required"].value == 0
    assert empty.not_enough_info
    assert not score_lead(ext(required_skills=["Go"]), job(), profile, cfg).not_enough_info


# M3-06 review: the extraction sometimes lists the employer as a skill ("Stripe" at
# Stripe). Dropped in code after validation, before storing and before scoring.
@pytest.mark.parametrize(
    ("company", "names", "want"),
    [
        ("Stripe", ["Stripe", "Go"], ["Go"]),
        ("Stripe", ["STRIPE", "stripe"], []),  # folded like skill names
        ("Stripe", ["Stripe API", "Go"], ["Stripe API", "Go"]),  # only exact names
        ("Monzo", ["Go"], ["Go"]),
    ],
)
def test_drop_company(company: str, names: list[str], want: list[str]) -> None:
    got = drop_company(ext(required_skills=names, optional_skills=names), company)
    assert got.required_skills == want
    assert got.optional_skills == want


# Scoring code and the tech-alias table are part of the weights version: changing either
# re-scores every lead (free), like a scoring.yaml edit.
def test_weights_version_covers_code_and_aliases(monkeypatch: pytest.MonkeyPatch) -> None:
    v = load_config().weights_version
    monkeypatch.setattr(score_module, "SCORE_CODE", score_module.SCORE_CODE + 1)
    assert load_config().weights_version != v
    monkeypatch.undo()
    monkeypatch.setitem(aliases.TABLE, "Brand new tech", ("bnt",))
    assert load_config().weights_version != v


# M3-09 AC1: location/remote/visa points below bands.location_gate cap the fit level at
# stretch; the total is unchanged. The same table is in internal/httpapi/score_test.go.
@pytest.mark.parametrize(
    ("total_points", "location_points", "want"),
    [
        (80, 3.0, "stretch"),  # very strong, but can't work there
        (80, 5.0, "very_strong"),  # at the gate: not capped
        (60, 4.9, "stretch"),
        (40, 0.0, "stretch"),  # good → stretch
        (32.5, 3.0, "stretch"),  # already stretch
        (10, 3.0, "no"),  # never raised
    ],
)
def test_location_gate_caps_the_level(
    cfg: Config, total_points: float, location_points: float, want: str
) -> None:
    assert fit_level(total_points, location_points, cfg.bands) == want


def test_gate_is_in_the_bands_section(cfg: Config) -> None:
    assert cfg.bands.location_gate == 5


# M3-09 AC3: the location detail names the place and the workplace, so a capped reason
# can say where ("elsewhere: on-site in Sao Paulo, visa unknown").
def test_location_detail_names_the_place(cfg: Config) -> None:
    j = job(workplace="onsite", countries=("BR",), locations=("Sao Paulo", "Rio de Janeiro"))
    f = location(ext(), j, cfg)
    assert f.detail == "elsewhere: on-site in Sao Paulo, visa unknown"
    hybrid = location(
        ext(visa_sponsorship="no"),
        job(workplace="hybrid", countries=("DE",), locations=("Berlin",)),
        cfg,
    )
    assert hybrid.detail == "relocation: hybrid in Berlin, visa no"
    unknown_wp = location(ext(), job(countries=("US",), locations=("New York",)), cfg)
    assert unknown_wp.detail == "elsewhere: in New York, visa unknown"
    # a location that only names the workplace is skipped ("Hybrid", "Remote")
    places = ("Hybrid", "Austin, TX")
    word = location(ext(), job(workplace="hybrid", countries=("US",), locations=places), cfg)
    assert word.detail == "elsewhere: hybrid in Austin, TX, visa unknown"


def test_scored_says_when_capped(profile: Profile, cfg: Config) -> None:
    far = score_lead(ext(required_skills=["PostgreSQL"]), job(countries=("US",)), profile, cfg)
    near = score_lead(
        ext(required_skills=["PostgreSQL"]),
        job(workplace="remote", remote_scopes=("worldwide",)),
        profile,
        cfg,
    )
    assert far.capped
    assert not near.capped


# #44 review (ADR-0023): with several locations, the detail names the one that passed the
# default preset (reconcile's matched_location), not the first.
@pytest.mark.parametrize(
    ("locations", "matched", "want"),
    [
        (("Austin, TX", "London"), "London", "elsewhere: hybrid in London, visa unknown"),
        (("Canada", "Greece"), "Greece", "elsewhere: hybrid in Greece, visa unknown"),
        (("Austin, TX", "London, UK"), "", "elsewhere: hybrid in Austin, TX, visa unknown"),  # none
        (("Hybrid", "Austin, TX"), "Hybrid", "elsewhere: hybrid in Austin, TX, visa unknown"),
    ],
)
def test_location_detail_uses_the_matched_location(
    cfg: Config, locations: tuple[str, ...], matched: str, want: str
) -> None:
    j = job(workplace="hybrid", countries=("US",), locations=locations, matched_location=matched)
    assert location(ext(), j, cfg).detail == want
