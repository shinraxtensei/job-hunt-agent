"""M3-01 AC2 and AC3 against a real Postgres and a real process: the worker connects as
the app role, and a real SIGTERM stops it cleanly within 10 s."""

import json
import os
import signal
import subprocess
import sys
import time
from pathlib import Path

import pytest

from jobhunt_agents.db import open_pool

pytestmark = pytest.mark.integration


def test_connects_as_app_role(app_dsn: str) -> None:
    with open_pool(app_dsn, max_size=2) as pool, pool.connection() as conn:
        row = conn.execute("SELECT current_user").fetchone()
    assert row == ("jobhunt_app",)


def start_worker(
    app_dsn: str, tmp_path: Path
) -> tuple[subprocess.Popen[str], list[dict[str, str]]]:
    env = {
        "PATH": os.environ["PATH"],
        "DATABASE_URL": app_dsn,
        "WORKER_POLL_SECONDS": "60",  # a long wait, so SIGTERM arrives while it waits
        "HEARTBEAT_FILE": str(tmp_path / "hb"),
    }
    proc = subprocess.Popen(
        [sys.executable, "-m", "jobhunt_agents"],
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
    )
    lines: list[dict[str, str]] = []
    assert proc.stdout is not None
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        line = proc.stdout.readline()
        if not line:
            break
        lines.append(json.loads(line))
        if lines[-1].get("msg") == "worker started":
            return proc, lines
    proc.kill()
    pytest.fail(f"worker never started: {lines}")


@pytest.mark.parametrize("sig", [signal.SIGTERM, signal.SIGINT])
def test_signal_stops_worker_cleanly(app_dsn: str, tmp_path: Path, sig: signal.Signals) -> None:
    proc, lines = start_worker(app_dsn, tmp_path)
    assert lines[-1]["user"] == "jobhunt_app"
    assert "test-app" not in json.dumps(lines)  # the password is never logged
    began = time.monotonic()
    proc.send_signal(sig)
    out, _ = proc.communicate(timeout=10)
    assert proc.returncode == 0
    assert time.monotonic() - began < 10
    rest = [json.loads(line) for line in out.splitlines() if line]
    assert any(r.get("msg") == "worker stopped" for r in rest)
