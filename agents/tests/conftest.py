"""A throwaway Postgres for integration tests: the image pinned in deploy/compose.yaml,
with the same role init script, like the Go tests (internal/store/storetest)."""

import re
from collections.abc import Iterator
from pathlib import Path

import pytest
from testcontainers.community.postgres import PostgresContainer

ROOT = Path(__file__).resolve().parents[2]
APP_PASSWORD = "test-app"


def compose_image() -> str:
    text = (ROOT / "deploy" / "compose.yaml").read_text()
    m = re.search(r"^\s*image:\s*(postgres:\S+)", text, re.MULTILINE)
    assert m, "no postgres image in deploy/compose.yaml"
    return m.group(1)


@pytest.fixture(scope="session")
def app_dsn() -> Iterator[str]:
    """A connection URL for the jobhunt_app role on a fresh database."""
    pg = (
        PostgresContainer(
            compose_image(), username="postgres", password="test-super", dbname="jobhunt"
        )
        .with_env("JOBHUNT_OWNER_PASSWORD", "test-owner")
        .with_env("JOBHUNT_APP_PASSWORD", APP_PASSWORD)
        .with_volume_mapping(
            str(ROOT / "deploy" / "postgres" / "initdb" / "01-roles.sh"),
            "/docker-entrypoint-initdb.d/01-roles.sh",
        )
    )
    with pg:
        host, port = pg.get_container_host_ip(), pg.get_exposed_port(5432)
        yield f"postgresql://jobhunt_app:{APP_PASSWORD}@{host}:{port}/jobhunt"
