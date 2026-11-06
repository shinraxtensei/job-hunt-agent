"""M3-01 AC3: the loop finishes the iteration in progress, then stops; the heartbeat
backs Compose's healthcheck."""

import json
import logging
import os
import threading
import time
from decimal import Decimal
from pathlib import Path

import pytest

from jobhunt_agents.__main__ import JSONFormatter
from jobhunt_agents.worker import Worker, healthy


class SlowIteration:
    """An iteration that takes a while, recording whether it ran to the end."""

    def __init__(self) -> None:
        self.started = threading.Event()
        self.finished = 0

    def __call__(self) -> bool:
        self.started.set()
        time.sleep(0.3)
        self.finished += 1
        return False


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

    def iterate() -> bool:
        calls.append(1)
        return False

    stop = threading.Event()
    w = Worker(iterate=iterate, poll_seconds=60, heartbeat=tmp_path / "hb")
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
    Worker(iterate=lambda: False, poll_seconds=1, heartbeat=hb).run(stop)
    assert hb.exists()
    assert healthy(hb, max_age_seconds=30)
    old = time.time() - 120
    os.utime(hb, (old, old))
    assert not healthy(hb, max_age_seconds=30)


# M3-02: while there is work, the next iteration starts at once (no poll wait).
def test_busy_iterations_do_not_wait(tmp_path: Path) -> None:
    stop = threading.Event()
    left = [3]

    def iterate() -> bool:
        left[0] -= 1
        if left[0] < 0:
            stop.set()
        return left[0] >= 0

    began = time.monotonic()
    Worker(iterate=iterate, poll_seconds=60, heartbeat=tmp_path / "hb").run(stop)
    assert time.monotonic() - began < 1


class Down(Exception):
    pass


# M3-02 design point 3: a transient error (the database) is retried; it becomes fatal
# only after lasting fatal_after_seconds; the heartbeat stops while it lasts.
def test_transient_errors_retry_then_become_fatal(tmp_path: Path) -> None:
    hb = tmp_path / "hb"
    calls = [0]

    def iterate() -> bool:
        calls[0] += 1
        raise Down

    w = Worker(
        iterate=iterate, poll_seconds=0.05, heartbeat=hb, transient=(Down,), fatal_after_seconds=0.3
    )
    with pytest.raises(Down):
        w.run(threading.Event())
    assert calls[0] > 2
    assert not hb.exists()


def test_transient_error_recovers(tmp_path: Path) -> None:
    stop = threading.Event()
    outcomes: list[bool | None] = [None, None, False]  # None: raise

    def iterate() -> bool:
        o = outcomes.pop(0)
        if not outcomes:
            stop.set()
        if o is None:
            raise Down
        return o

    hb = tmp_path / "hb"
    Worker(
        iterate=iterate, poll_seconds=0.01, heartbeat=hb, transient=(Down,), fatal_after_seconds=5
    ).run(stop)
    assert hb.exists()


def test_other_errors_are_not_swallowed(tmp_path: Path) -> None:
    def iterate() -> bool:
        raise KeyError("bug")

    w = Worker(iterate=iterate, poll_seconds=0.01, heartbeat=tmp_path / "hb", transient=(Down,))
    with pytest.raises(KeyError):
        w.run(threading.Event())


# M3-06: the model client logs its cost as a Decimal. The JSON formatter must write it:
# when formatting fails, logging prints the active exception, which can hold model output.
def test_json_log_formats_decimal() -> None:
    rec = logging.LogRecord("x", logging.INFO, __file__, 1, "model call", None, None)
    rec.cost = Decimal("0.000123")
    assert json.loads(JSONFormatter().format(rec))["cost"] == "0.000123"
