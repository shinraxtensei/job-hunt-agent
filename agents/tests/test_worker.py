"""M3-01 AC3: the loop finishes the iteration in progress, then stops; the heartbeat
backs Compose's healthcheck."""

import os
import threading
import time
from pathlib import Path

from jobhunt_agents.worker import Worker, healthy


class SlowIteration:
    """An iteration that takes a while, recording whether it ran to the end."""

    def __init__(self) -> None:
        self.started = threading.Event()
        self.finished = 0

    def __call__(self) -> None:
        self.started.set()
        time.sleep(0.3)
        self.finished += 1


def test_stop_during_iteration_finishes_it(tmp_path: Path) -> None:
    work = SlowIteration()
    stop = threading.Event()
    w = Worker(iterate=work, poll_seconds=60, heartbeat=tmp_path / "hb")
    t = threading.Thread(target=w.run, args=(stop,))
    t.start()
    assert work.started.wait(5)
    stop.set()  # mid-iteration
    t.join(5)
    assert not t.is_alive()
    assert work.finished == 1  # the iteration completed; no new one began


def test_stop_while_waiting_returns_at_once(tmp_path: Path) -> None:
    calls: list[int] = []
    stop = threading.Event()
    w = Worker(iterate=lambda: calls.append(1), poll_seconds=60, heartbeat=tmp_path / "hb")
    t = threading.Thread(target=w.run, args=(stop,))
    t.start()
    time.sleep(0.2)
    began = time.monotonic()
    stop.set()
    t.join(5)
    assert not t.is_alive()
    assert time.monotonic() - began < 1  # the 60 s wait is interrupted
    assert calls == [1]


def test_heartbeat_and_healthcheck(tmp_path: Path) -> None:
    hb = tmp_path / "hb"
    assert not healthy(hb, max_age_seconds=30)  # never written
    stop = threading.Event()
    stop.set()
    Worker(iterate=lambda: None, poll_seconds=1, heartbeat=hb).run(stop)
    assert hb.exists()
    assert healthy(hb, max_age_seconds=30)
    old = time.time() - 120
    os.utime(hb, (old, old))
    assert not healthy(hb, max_age_seconds=30)
