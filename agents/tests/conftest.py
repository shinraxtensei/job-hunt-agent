"""A throwaway Postgres for integration tests: the image pinned in deploy/compose.yaml,
with the same role init script and the same migrations (applied by the Go migrate
command), like the Go tests (internal/store/storetest)."""

import os
import re
import subprocess
import uuid
from collections.abc import Iterator
from pathlib import Path

import psycopg
import pytest
from testcontainers.community.postgres import PostgresContainer

ROOT = Path(__file__).resolve().parents[2]
APP_PASSWORD = "test-app"
OWNER_PASSWORD = "test-owner"


def compose_image() -> str:
    text = (ROOT / "deploy" / "compose.yaml").read_text()
    m = re.search(r"^\s*image:\s*(postgres:\S+)", text, re.MULTILINE)
    assert m, "no postgres image in deploy/compose.yaml"
    return m.group(1)


@pytest.fixture(scope="session")
def go_bin(tmp_path_factory: pytest.TempPathFactory) -> Path:
    """The Go migrate and collector commands, built once from this checkout."""
    out = tmp_path_factory.mktemp("bin")
    for cmd in ("migrate", "collector"):
        subprocess.run(["go", "build", "-o", str(out / cmd), f"./cmd/{cmd}"], cwd=ROOT, check=True)
    return out


@pytest.fixture(scope="session")
def app_dsn(go_bin: Path) -> Iterator[str]:
    """A connection URL for the jobhunt_app role on a migrated database."""
    pg = (
        PostgresContainer(
            compose_image(), username="postgres", password="test-super", dbname="jobhunt"
        )
        .with_env("JOBHUNT_OWNER_PASSWORD", OWNER_PASSWORD)
        .with_env("JOBHUNT_APP_PASSWORD", APP_PASSWORD)
        .with_volume_mapping(
            str(ROOT / "deploy" / "postgres" / "initdb" / "01-roles.sh"),
            "/docker-entrypoint-initdb.d/01-roles.sh",
        )
    )
    with pg:
        host, port = pg.get_container_host_ip(), pg.get_exposed_port(5432)
        base = f"{host}:{port}/jobhunt"
        owner = f"postgres://jobhunt_owner:{OWNER_PASSWORD}@{base}?sslmode=disable"
        subprocess.run(
            [str(go_bin / "migrate"), "up"],
            env={"PATH": os.environ["PATH"], "MIGRATIONS_DATABASE_URL": owner},
            check=True,
            capture_output=True,
        )
        yield f"postgresql://jobhunt_app:{APP_PASSWORD}@{base}"


class Go:
    """The Go side of the queue: add jobs by link, which enqueues what the default
    preset shows (the reconcile step)."""

    def __init__(self, bin_dir: Path, dsn: str) -> None:
        self._collector = bin_dir / "collector"
        self._dsn = dsn

    def add_shown_job(self) -> int:
        """Adds a manual remote backend job (shown by the default preset); its id."""
        out = subprocess.run(
            [
                str(self._collector),
                "add",
                f"https://jobs.example.test/{uuid.uuid4()}",
                "--title",
                "Backend Engineer",
                "--company",
                f"Contract Co {uuid.uuid4().hex[:8]}",  # its own group
                "--location",
                "Remote",
            ],
            cwd=ROOT,  # data/presets
            env={"PATH": os.environ["PATH"], "DATABASE_URL": self._dsn},
            check=True,
            capture_output=True,
            text=True,
        ).stdout
        m = re.search(r"id (\d+)", out)
        assert m, out
        return int(m.group(1))


@pytest.fixture
def go(go_bin: Path, app_dsn: str) -> Go:
    """The Go side, with no runnable items left by earlier tests (tests claim whatever is
    runnable). Marked done, not deleted: reconcile would queue those postings again."""
    with psycopg.connect(app_dsn, autocommit=True) as conn:
        conn.execute(
            "UPDATE work_items SET state = 'done', locked_until = NULL, claim_token = NULL"
        )
    return Go(go_bin, app_dsn)
