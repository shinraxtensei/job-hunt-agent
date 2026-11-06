"""M3-06 AC1, AC4, AC5: coded scores stored for shown leads with their breakdown and
versions, recomputed with no model call when weights or the profile change, and reason
items queued lazily. Real Postgres; no model provider anywhere in this file."""

import json
import shutil
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

import psycopg
import pytest
from psycopg.types.json import Jsonb

from jobhunt_agents.db import open_pool
from jobhunt_agents.extraction import Extraction
from jobhunt_agents.profile import Profile, load
from jobhunt_agents.rescore import rescore
from jobhunt_agents.score import (
    JOB_SQL,
    Config,
    fit_level,
    job_from_row,
    load_config,
    score,
    total,
)
from tests.conftest import ROOT, Go

EXTRACTION: dict[str, Any] = {
    "required_skills": ["PostgreSQL", "Kubernetes", "Rust"],
    "optional_skills": ["Go"],
    "min_years": 2,
    "seniority": "mid",
    "visa_sponsorship": "unknown",
    "salary": None,
    "languages": [],
    "remote_policy": "remote",
}
SCORING = ROOT / "agents" / "src" / "jobhunt_agents" / "scoring.yaml"


@pytest.fixture
def profile_dir(tmp_path: Path) -> Path:
    for name in ("skill_map", "search_profile"):
        shutil.copy(ROOT / "data" / "profile" / f"{name}.example.yaml", tmp_path / f"{name}.yaml")
    return tmp_path


def extracted(dsn: str, posting: int, data: dict[str, Any] = EXTRACTION) -> None:
    """An extraction of the posting's current content, as the worker stores it."""
    with psycopg.connect(dsn) as conn:
        conn.execute(
            "INSERT INTO extractions (posting_id, content_hash, prompt_version, model, data)"
            " SELECT id, content_hash, 2, 'claude-haiku-5-5', %s FROM postings WHERE id = %s",
            (Jsonb(data), posting),
        )


def row(dsn: str, posting: int) -> dict[str, Any] | None:
    with psycopg.connect(dsn) as conn:
        cur = conn.execute("SELECT * FROM scores WHERE posting_id = %s", (posting,))
        r = cur.fetchone()
        if r is None:
            return None
        assert cur.description
        return {d.name: v for d, v in zip(cur.description, r, strict=True)}


def reason_item(dsn: str, posting: int) -> str | None:
    with psycopg.connect(dsn) as conn:
        r = conn.execute(
            "SELECT state FROM work_items WHERE posting_id = %s AND kind = 'reason'", (posting,)
        ).fetchone()
    return r[0] if r else None


def model_calls(dsn: str) -> int:
    with psycopg.connect(dsn) as conn:
        r = conn.execute("SELECT count(*) FROM model_calls").fetchone()
    assert r
    return int(r[0])


def expected(dsn: str, posting: int, profile: Profile, cfg: Config) -> float:
    with psycopg.connect(dsn) as conn:
        j = conn.execute(JOB_SQL, (posting,)).fetchone()
    assert j
    return score(Extraction.model_validate(EXTRACTION), job_from_row(j), profile, cfg).points


# AC1: a shown lead with an extraction gets a stored score: the code's weighted sum, the
# factor breakdown and the versions it came from; a reason item is queued for it.
def test_shown_lead_is_scored(app_dsn: str, go: Go, profile_dir: Path) -> None:
    posting = go.add_shown_job()
    extracted(app_dsn, posting)
    profile, cfg = load(profile_dir), load_config()
    with open_pool(app_dsn, max_size=2) as pool:
        assert rescore(pool, profile, cfg) >= 1
    got = row(app_dsn, posting)
    assert got is not None
    assert got["points"] == pytest.approx(expected(app_dsn, posting, profile, cfg))
    factors: dict[str, Any] = got["factors"]
    assert set(factors) == {"required", "title", "seniority", "location", "optional"}
    # M3-08: each factor carries its weight, so the API shows points without the config
    assert {k: f["weight"] for k, f in factors.items()} == {
        k: getattr(cfg.weights, k) for k in factors
    }
    assert [m["level"] for m in factors["required"]["matches"]] == ["proven", "familiar", "missing"]
    assert (got["weights_version"], got["bands_version"], got["profile_version"]) == (
        cfg.weights_version,
        cfg.bands_version,
        profile.version,
    )
    assert got["extraction_prompt_version"] == 2
    assert got["not_enough_info"] is False
    assert reason_item(app_dsn, posting) == "pending"


# A lead without an extraction of its current content is not scored.
def test_no_extraction_no_score(app_dsn: str, go: Go, profile_dir: Path) -> None:
    posting = go.add_shown_job()
    with open_pool(app_dsn, max_size=2) as pool:
        rescore(pool, load(profile_dir), load_config())
    assert row(app_dsn, posting) is None


# AC4: a weights or credit change recomputes every shown lead at once, with zero model
# calls; unchanged inputs are not rewritten.
def test_config_change_rescores_with_no_model_call(
    app_dsn: str, go: Go, profile_dir: Path, tmp_path: Path
) -> None:
    a, b = go.add_shown_job(), go.add_shown_job()
    extracted(app_dsn, a)
    extracted(app_dsn, b)
    profile = load(profile_dir)
    calls = model_calls(app_dsn)
    changed = tmp_path / "scoring.yaml"
    changed.write_text(SCORING.read_text().replace("familiar: 0.5", "familiar: 0.9"))
    with open_pool(app_dsn, max_size=2) as pool:
        rescore(pool, profile, load_config())
        assert rescore(pool, profile, load_config()) == 0  # nothing changed: no writes
        before = row(app_dsn, a)
        n = rescore(pool, profile, load_config(changed))
    after = row(app_dsn, a)
    assert before is not None
    assert after is not None
    assert n >= 2
    assert after["points"] > before["points"]  # Kubernetes is familiar
    assert after["weights_version"] != before["weights_version"]
    assert row(app_dsn, b) is not None
    assert model_calls(app_dsn) == calls


# The view adds freshness as of now and the fit level from the stored bands, as score.py
# does.
# M3-09: with low bands so the total is "very strong", an on-site job abroad is capped
# at stretch by the location gate, in the view as in the code.
@pytest.mark.parametrize("abroad", [False, True])
def test_view_matches_the_code(
    app_dsn: str, go: Go, profile_dir: Path, tmp_path: Path, abroad: bool
) -> None:
    posting = go.add_shown_job()
    extracted(app_dsn, posting, EXTRACTION | ({"remote_policy": "onsite"} if abroad else {}))
    with psycopg.connect(app_dsn) as conn:  # part of a day: freshness counts whole days
        conn.execute(
            "UPDATE jobs SET published_at = now() - interval '26 days 20 hours'"
            " WHERE posting_id = %s",
            (posting,),
        )
        if abroad:
            conn.execute(
                "UPDATE jobs SET workplace = 'onsite', countries = '{US}', remote_scopes = '{}',"
                " locations = '{Austin}' WHERE posting_id = %s",
                (posting,),
            )
    low = tmp_path / "scoring.yaml"
    low.write_text(
        SCORING.read_text()
        .replace("very_strong: 73", "very_strong: 30")
        .replace("strong: 53", "strong: 20")
        .replace("good: 33", "good: 10")
        .replace("stretch: 32", "stretch: 5")
    )
    profile, cfg = load(profile_dir), load_config(low)
    with open_pool(app_dsn, max_size=2) as pool:
        rescore(pool, profile, cfg)
    with psycopg.connect(app_dsn) as conn:
        v = conn.execute(
            "SELECT total, fit_level, reason_current FROM lead_scores WHERE posting_id = %s",
            (posting,),
        ).fetchone()
        j = conn.execute(JOB_SQL, (posting,)).fetchone()
    assert v
    assert j
    job = job_from_row(j)
    e = Extraction.model_validate(EXTRACTION | ({"remote_policy": "onsite"} if abroad else {}))
    s = score(e, job, profile, cfg)
    want = total(s, job.published_at, datetime.now(UTC), cfg)
    assert v[0] == pytest.approx(want, abs=0.01)
    loc = s.factors["location"].value * cfg.weights.location
    assert v[1] == fit_level(want, loc, cfg.bands)
    assert v[1] == ("stretch" if abroad else "very_strong")
    assert v[2] is False  # no reason yet


def stored_reason(dsn: str, posting: int) -> None:
    """A reason for the score's current inputs, as the reason handler stores it."""
    with psycopg.connect(dsn) as conn:
        conn.execute(
            "INSERT INTO score_reasons (posting_id, content_hash, inputs, prompt_version, model,"
            " reason, gaps, cv_variant) SELECT posting_id, content_hash, reason_inputs, 1, 'm',"
            " 'Fits.', '[]', 'BE' FROM scores WHERE posting_id = %s",
            (posting,),
        )
        conn.execute(
            "UPDATE work_items SET state = 'done' WHERE posting_id = %s AND kind = 'reason'",
            (posting,),
        )


# AC5: a profile change recomputes the score but not the reason: the old one stays,
# marked not current, until the lead is opened (reason_requested_at); new posting content
# queues a reason at once.
def test_reason_regenerated_lazily(app_dsn: str, go: Go, profile_dir: Path) -> None:
    posting = go.add_shown_job()
    extracted(app_dsn, posting)
    cfg = load_config()
    with open_pool(app_dsn, max_size=2) as pool:
        rescore(pool, load(profile_dir), cfg)
        stored_reason(app_dsn, posting)
        sm = profile_dir / "skill_map.yaml"
        sm.write_text(
            sm.read_text().replace(
                "level: familiar\n    aliases: [K8s]",
                "level: proven\n    evidence: x\n    aliases: [K8s]",
            )
        )
        rescore(pool, load(profile_dir), cfg)
        assert reason_item(app_dsn, posting) == "done"  # not regenerated yet
        with psycopg.connect(app_dsn) as conn:
            cur = conn.execute(
                "SELECT reason, reason_current FROM lead_scores WHERE posting_id = %s", (posting,)
            ).fetchone()
            assert cur == ("Fits.", False)  # shown, marked older
            conn.execute(
                "UPDATE scores SET reason_requested_at = now() WHERE posting_id = %s", (posting,)
            )
        rescore(pool, load(profile_dir), cfg)
        assert reason_item(app_dsn, posting) == "pending"  # opened: regenerated

        stored_reason_done = "UPDATE work_items SET state = 'done' WHERE posting_id = %s"
        with psycopg.connect(app_dsn) as conn:
            conn.execute(stored_reason_done, (posting,))
            conn.execute(
                "UPDATE scores SET reason_requested_at = NULL WHERE posting_id = %s", (posting,)
            )
            conn.execute(
                "UPDATE score_reasons SET inputs = (SELECT reason_inputs FROM scores"
                " WHERE posting_id = %s) WHERE posting_id = %s",
                (posting, posting),
            )
            # the posting changes: new content, extracted again
            conn.execute(
                "UPDATE postings SET content_hash = sha256(content_hash) WHERE id = %s", (posting,)
            )
        extracted(app_dsn, posting)
        rescore(pool, load(profile_dir), cfg)
    assert reason_item(app_dsn, posting) == "pending"
    got = row(app_dsn, posting)
    assert got is not None
    with psycopg.connect(app_dsn) as conn:
        h = conn.execute("SELECT content_hash FROM postings WHERE id = %s", (posting,)).fetchone()
    assert h
    assert got["content_hash"] == h[0]


# Not enough info: no required skills stated → the flag is stored.
def test_not_enough_info_stored(app_dsn: str, go: Go, profile_dir: Path) -> None:
    posting = go.add_shown_job()
    extracted(app_dsn, posting, EXTRACTION | {"required_skills": []})
    with open_pool(app_dsn, max_size=2) as pool:
        rescore(pool, load(profile_dir), load_config())
    got = row(app_dsn, posting)
    assert got is not None
    assert got["not_enough_info"] is True
    assert json.dumps(got["factors"])  # stored as JSON


# M3-06 review: a skill named like the posting's company is dropped before scoring, also
# for extractions stored before the filter existed (no re-extraction).
def test_company_name_is_not_a_skill(app_dsn: str, go: Go, profile_dir: Path) -> None:
    posting = go.add_shown_job()
    with psycopg.connect(app_dsn) as conn:
        r = conn.execute(
            "SELECT c.name FROM jobs j JOIN companies c ON c.id = j.company_id"
            " WHERE j.posting_id = %s",
            (posting,),
        ).fetchone()
    assert r
    extracted(app_dsn, posting, EXTRACTION | {"required_skills": ["PostgreSQL", r[0]]})
    with open_pool(app_dsn, max_size=2) as pool:
        rescore(pool, load(profile_dir), load_config())
    got = row(app_dsn, posting)
    assert got is not None
    assert [m["name"] for m in got["factors"]["required"]["matches"]] == ["PostgreSQL"]
