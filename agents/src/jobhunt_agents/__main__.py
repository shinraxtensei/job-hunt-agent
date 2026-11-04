"""Entrypoint: `python -m jobhunt_agents` runs the worker; `... healthcheck` checks it."""

import json
import logging
import signal
import sys
import threading
from types import FrameType

from jobhunt_agents.db import open_pool
from jobhunt_agents.settings import ConfigError, Settings, load
from jobhunt_agents.worker import Worker, healthy

log = logging.getLogger("jobhunt_agents")


class JSONFormatter(logging.Formatter):
    """One JSON object per line; only the message and explicit `extra` fields."""

    FIELDS = ("user", "poll_seconds", "error")

    def format(self, record: logging.LogRecord) -> str:
        out: dict[str, object] = {
            "time": self.formatTime(record, "%Y-%m-%dT%H:%M:%S%z"),
            "level": record.levelname,
            "msg": record.getMessage(),
        }
        out.update({k: getattr(record, k) for k in self.FIELDS if hasattr(record, k)})
        return json.dumps(out)


def setup_logging(level: str) -> None:
    h = logging.StreamHandler(sys.stdout)
    h.setFormatter(JSONFormatter())
    logging.basicConfig(level=level, handlers=[h], force=True)
    logging.getLogger("psycopg").setLevel(logging.WARNING)


def run(s: Settings) -> None:
    stop = threading.Event()

    def on_signal(_sig: int, _frame: FrameType | None) -> None:
        stop.set()

    signal.signal(signal.SIGTERM, on_signal)
    signal.signal(signal.SIGINT, on_signal)

    with open_pool(s.database_url.get_secret_value(), max_size=s.worker_pool_max) as pool:
        with pool.connection() as conn:
            row = conn.execute("SELECT current_user").fetchone()
        user = row[0] if row else None
        log.info("worker started", extra={"user": user, "poll_seconds": s.worker_poll_seconds})

        def iterate() -> None:
            # No queue yet (M3-02): prove the database answers.
            with pool.connection() as c:
                c.execute("SELECT 1")

        Worker(iterate=iterate, poll_seconds=s.worker_poll_seconds, heartbeat=s.heartbeat_file).run(
            stop
        )
    log.info("worker stopped")


def main(argv: list[str] | None = None) -> int:
    args = sys.argv[1:] if argv is None else argv
    setup_logging("INFO")
    try:
        s = load()
    except ConfigError as e:
        log.error("worker cannot start", extra={"error": str(e)})  # noqa: TRY400 - no traceback: it would be noise
        return 2
    setup_logging(s.log_level)
    if args == ["healthcheck"]:
        # Iterations run every poll_seconds; allow three missed beats plus slack.
        return 0 if healthy(s.heartbeat_file, max_age_seconds=3 * s.worker_poll_seconds + 10) else 1
    if args:
        log.error("unknown arguments; usage: python -m jobhunt_agents [healthcheck]")
        return 2
    run(s)
    return 0


if __name__ == "__main__":
    sys.exit(main())
