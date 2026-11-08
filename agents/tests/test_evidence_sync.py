"""M4-01 AC2: the bank is synced into Postgres with its version; a line removed from the
file is retired, never deleted (drafts that cite it stay checkable). Real Postgres."""

import shutil
from pathlib import Path
from typing import Any, LiteralString

import psycopg
import pytest

from jobhunt_agents.db import open_pool
from jobhunt_agents.evidence import load_evidence
from jobhunt_agents.evidence_sync import sync
from jobhunt_agents.profile import Profile, load
from tests.conftest import ROOT

EXAMPLE = ROOT / "data" / "profile" / "evidence.example.yaml"


@pytest.fixture
def profile(tmp_path: Path) -> Profile:
    d = tmp_path / "profile"
    d.mkdir()
    for name in ("skill_map", "search_profile"):
        shutil.copy(ROOT / "data" / "profile" / f"{name}.example.yaml", d / f"{name}.yaml")
    return load(d)


def rows(dsn: str, sql: LiteralString) -> list[tuple[Any, ...]]:
    with psycopg.connect(dsn) as conn:
        return conn.execute(sql).fetchall()


def test_sync_stores_and_retires(app_dsn: str, profile: Profile, tmp_path: Path) -> None:
    bank = load_evidence(EXAMPLE, profile)
    with psycopg.connect(app_dsn) as conn:  # the session database: other tests sync it too
        conn.execute("UPDATE evidence_bank SET version = 'another'")
    with open_pool(app_dsn, max_size=2) as pool:
        assert sync(pool, bank) is True
        assert sync(pool, bank) is False  # same version: nothing written
        lines = rows(app_dsn, "SELECT id, item_id, retired FROM evidence_lines ORDER BY position")
        assert ("example-billing", "example-corp", False) in lines
        assert ("summary-years", "summary", False) in lines
        [(numbers,)] = rows(
            app_dsn, "SELECT numbers FROM evidence_lines WHERE id = 'example-billing'"
        )
        assert numbers == [
            {"value": 40.0, "unit": "", "approx": False, "plus": False},
            {"value": 6.0, "unit": "minute", "approx": False, "plus": False},
        ]
        [(skills,)] = rows(
            app_dsn, "SELECT skills FROM evidence_lines WHERE id = 'side-project-collector'"
        )
        assert {"name": "Go", "skill": "Go", "level": "learning"} in skills
        [(facts,)] = rows(app_dsn, "SELECT facts FROM evidence_items WHERE id = 'example-corp'")
        assert facts["title"] == "Backend Engineer"
        assert "lines" not in facts

        # the owner removes a line and edits another
        text = EXAMPLE.read_text()
        start = text.index("      - id: example-deploys")
        end = text.index("projects:")
        edited = tmp_path / "evidence.yaml"
        edited.write_text(text[:start] + "\n" + text[end:])
        sync(pool, load_evidence(edited, profile))
    after = dict(rows(app_dsn, "SELECT id, retired FROM evidence_lines"))
    assert after["example-deploys"] is True  # kept, retired
    assert after["example-billing"] is False
    [(version,)] = rows(app_dsn, "SELECT version FROM evidence_bank")
    assert version != bank.version
