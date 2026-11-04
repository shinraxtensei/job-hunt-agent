"""The worker loop (M3-01 AC3, M3-02): run one iteration, wait, repeat, until stopped.

A stop request never interrupts an iteration: it is checked between iterations and
wakes the wait early. An iteration that found work is followed at once by the next.
"""

import logging
import threading
import time
from collections.abc import Callable
from pathlib import Path

log = logging.getLogger(__name__)


class Worker:
    def __init__(
        self,
        *,
        iterate: Callable[[], bool],
        poll_seconds: float,
        heartbeat: Path,
        transient: tuple[type[Exception], ...] = (),
        fatal_after_seconds: float = 60,
    ) -> None:
        self._iterate = iterate
        self._poll = poll_seconds
        self._heartbeat = heartbeat
        self._transient = transient
        self._fatal_after = fatal_after_seconds

    def run(self, stop: threading.Event) -> None:
        """Returns when stop is set; raises a transient error that lasted too long."""
        failing_since: float | None = None
        while True:
            busy = False
            try:
                busy = self._iterate()
            except self._transient as e:
                now = time.monotonic()
                failing_since = failing_since or now
                if now - failing_since >= self._fatal_after:
                    raise
                log.warning("iteration failed; retrying", extra={"error": type(e).__name__})
            else:
                failing_since = None
                self._heartbeat.touch()  # only healthy while iterations succeed
            if stop.wait(0 if busy else self._poll):  # True as soon as stop is set
                return


def healthy(heartbeat: Path, *, max_age_seconds: float) -> bool:
    try:
        return time.time() - heartbeat.stat().st_mtime <= max_age_seconds
    except FileNotFoundError:
        return False
