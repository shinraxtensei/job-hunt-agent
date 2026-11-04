"""The worker loop (M3-01 AC3): run one iteration, wait, repeat, until stopped.

A stop request never interrupts an iteration: it is checked between iterations and
wakes the wait early.
"""

import threading
import time
from collections.abc import Callable
from pathlib import Path


class Worker:
    def __init__(
        self, *, iterate: Callable[[], None], poll_seconds: float, heartbeat: Path
    ) -> None:
        self._iterate = iterate
        self._poll = poll_seconds
        self._heartbeat = heartbeat

    def run(self, stop: threading.Event) -> None:
        while True:
            self._iterate()
            self._heartbeat.touch()
            if stop.wait(self._poll):  # True as soon as stop is set
                return


def healthy(heartbeat: Path, *, max_age_seconds: float) -> bool:
    try:
        return time.time() - heartbeat.stat().st_mtime <= max_age_seconds
    except FileNotFoundError:
        return False
