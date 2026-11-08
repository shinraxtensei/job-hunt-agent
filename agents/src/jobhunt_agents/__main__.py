"""Entrypoint: `python -m jobhunt_agents` runs the worker; `... healthcheck` checks it."""

import json
import logging
import signal
import sys
import threading
from datetime import UTC, date, datetime, timedelta
from types import FrameType

import psycopg

from jobhunt_agents import extract, reason, tailor
from jobhunt_agents.db import open_pool
from jobhunt_agents.evidence import EvidenceWatcher
from jobhunt_agents.evidence_sync import sync as sync_evidence
from jobhunt_agents.model import AnthropicProvider, ModelClient
from jobhunt_agents.profile import FILES, ProfileError, ProfileWatcher
from jobhunt_agents.queue import Handler, process_one
from jobhunt_agents.rescore import rescore
from jobhunt_agents.score import load_config
from jobhunt_agents.settings import ConfigError, Settings, load
from jobhunt_agents.worker import Worker, healthy

log = logging.getLogger("jobhunt_agents")


class JSONFormatter(logging.Formatter):
    """One JSON object per line; only the message and explicit `extra` fields."""

    FIELDS = (
        "user",
        "poll_seconds",
        "error",
        "item_id",
        "kind",
        "attempt",
        "agent",
        "outcome",
        "cost",
        "budget_usd",
        "version",
        "count",
    )

    def format(self, record: logging.LogRecord) -> str:
        out: dict[str, object] = {
            "time": self.formatTime(record, "%Y-%m-%dT%H:%M:%S%z"),
            "level": record.levelname,
            "msg": record.getMessage(),
        }
        out.update({k: getattr(record, k) for k in self.FIELDS if hasattr(record, k)})
        return json.dumps(out, default=str)  # Decimal costs; never fail: see the test


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

        lease = timedelta(seconds=s.worker_lease_seconds)
        scoring = load_config()  # scoring.yaml ships in the image: a change means a restart
        client = None
        # What the worker does, by work item kind; with no handler, a kind's items wait.
        handlers: dict[str, Handler] = {}
        if s.anthropic_api_key is not None:
            client = ModelClient(
                provider=AnthropicProvider(s.anthropic_api_key.get_secret_value()),
                pool=pool,
                daily_budget_usd=s.model_daily_budget_usd,
            )
            handlers["extract"] = extract.handler(pool, extract.Extractor(client))
            handlers["reason"] = reason.handler(pool, reason.Reasoner(client), scoring)
            extract.register_prompt(pool)
        else:
            log.info("no ANTHROPIC_API_KEY; extract, reason and tailor items wait")
        # M3-05: reloaded when the files change; M3-06 scores against profile.current().
        profile = None
        if all((s.profile_dir / f).is_file() for f in FILES):
            profile = ProfileWatcher(s.profile_dir)  # an invalid profile stops start-up
            log.info("profile loaded", extra={"version": profile.current().version[:12]})
        else:
            log.info("no profile in PROFILE_DIR; scoring stays off")
        evidence = EvidenceWatcher(s.profile_dir / "evidence.yaml")
        if client is not None and profile is not None:  # M4-02: drafts need both
            handlers["tailor"] = tailor.handler(
                pool, tailor.Tailorer(client), profile, evidence, s.profile_dir / "cv_layout.yaml"
            )
        paused_on: list[date] = []

        def iterate() -> bool:
            if profile is not None:
                # Coded scores, no model call (M3-06 AC4): only leads whose inputs changed;
                # a profile edit (picked up without a restart) re-scores every one.
                n = rescore(pool, profile.current(), scoring)
                if n:
                    log.info("scored", extra={"count": n})
                # M4-01: the evidence bank, synced when the file or the skill map changes
                bank = evidence.current(profile.current())
                if bank is not None and sync_evidence(pool, bank):
                    log.info("evidence synced", extra={"version": bank.version[:12]})
                tailor.queue_drafts(pool)  # M4-02: requested drafts get a tailor item
            # The daily budget (ADR-0018): stop claiming until the next UTC day.
            if client is not None and client.budget_exhausted():
                today = datetime.now(UTC).date()
                if paused_on != [today]:
                    paused_on[:] = [today]
                    log.warning(
                        "daily model budget reached; not claiming work until 00:00 UTC",
                        extra={"budget_usd": str(s.model_daily_budget_usd)},
                    )
                return False
            return process_one(pool, handlers, lease)

        Worker(
            iterate=iterate,
            poll_seconds=s.worker_poll_seconds,
            heartbeat=s.heartbeat_file,
            transient=(psycopg.OperationalError,),
            fatal_after_seconds=s.worker_db_outage_seconds,
        ).run(stop)
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
    try:
        run(s)
    except ProfileError as e:
        log.error("profile invalid; fix it and restart", extra={"error": str(e)})  # noqa: TRY400
        return 2
    except psycopg.OperationalError as e:
        # Fatal: Compose restarts the worker.
        log.error("database unreachable; exiting", extra={"error": type(e).__name__})  # noqa: TRY400
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
