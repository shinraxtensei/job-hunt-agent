"""M3-02: the work queue on a real Postgres, items enqueued by the real Go side."""

import threading
from datetime import timedelta
from pathlib import Path

import psycopg
import pytest
from psycopg.rows import dict_row

from jobhunt_agents.db import open_pool
from jobhunt_agents.queue import Item, ItemError, claim, complete, fail, process_one
from tests.conftest import ROOT, Go

pytestmark = pytest.mark.integration

LEASE = timedelta(minutes=5)


def contract_columns() -> list[str]:
    text = (ROOT / "db" / "contract" / "work_items.columns").read_text()
    return [ln.strip() for ln in text.splitlines() if ln.strip() and not ln.startswith("#")]


def row(dsn: str, item_id: int) -> dict[str, object]:
    with psycopg.connect(dsn) as conn, conn.cursor(row_factory=dict_row) as cur:
        cur.execute(
            "SELECT state, attempts, last_error, claim_token, run_after - now() AS wait"
            " FROM work_items WHERE id = %s",
            (item_id,),
        )
        r = cur.fetchone()
    assert r is not None
    return r


def claim_one(dsn: str, lease: timedelta = LEASE) -> Item:
    with psycopg.connect(dsn) as conn:
        item = claim(conn, ["extract"], lease)
    assert item is not None
    return item


# Contract (ADR-0001): what Go migrates is what Python reads; the Go side checks the same
# file (internal/store TestWorkItemsContract).
def test_work_items_contract(app_dsn: str, go: Go) -> None:
    with psycopg.connect(app_dsn) as conn:
        got = [
            f"{n} {t} {null}"
            for n, t, null in conn.execute(
                "SELECT column_name, data_type, is_nullable FROM information_schema.columns"
                " WHERE table_name = 'work_items' ORDER BY ordinal_position"
            )
        ]
    assert got == contract_columns()
    # ... and an item Go enqueued is claimable here, for the posting Go added.
    posting = go.add_shown_job()
    item = claim_one(app_dsn)
    assert (item.posting_id, item.kind, item.attempts) == (posting, "extract", 1)
    assert len(item.content_hash) == 32  # sha256


# AC2: several workers claiming at once each get distinct items, every item once.
def test_concurrent_claims_are_exclusive(app_dsn: str, go: Go) -> None:
    want = {go.add_shown_job() for _ in range(8)}
    claimed: list[int] = []
    lock = threading.Lock()
    start = threading.Barrier(4)

    def worker() -> None:
        with psycopg.connect(app_dsn, autocommit=True) as conn:
            start.wait()
            while (item := claim(conn, ["extract"], LEASE)) is not None:
                with lock:
                    claimed.append(item.posting_id)

    threads = [threading.Thread(target=worker) for _ in range(4)]
    for t in threads:
        t.start()
    for t in threads:
        t.join(30)
    assert sorted(claimed) == sorted(want)  # no item twice, none missed


# AC2, AC4, fencing: a worker dies between claim and ack; after its lease expires the
# item is claimed again with a new token; the dead worker's late ack is rejected, so the
# item is completed exactly once.
def test_expired_lease_is_reclaimed_and_late_ack_rejected(app_dsn: str, go: Go) -> None:
    go.add_shown_job()
    first = claim_one(app_dsn, lease=timedelta(seconds=1))
    with psycopg.connect(app_dsn) as conn:  # nothing to claim while the lease holds
        assert claim(conn, ["extract"], LEASE) is None
        conn.execute("UPDATE work_items SET locked_until = now() - interval '1 second'")
    second = claim_one(app_dsn)
    assert second.id == first.id
    assert second.claim_token != first.claim_token
    assert second.attempts == 2
    with psycopg.connect(app_dsn) as conn:
        assert not complete(conn, first)  # the late ack
        assert not fail(conn, first, "late")
        assert complete(conn, second)
        assert not complete(conn, second)  # once only
    r = row(app_dsn, first.id)
    assert (r["state"], r["attempts"], r["claim_token"]) == ("done", 2, None)


# AC3: a failing item is retried with growing backoff, then marked failed with the last
# error; the error holds no exception message (it could quote posting text).
def test_failure_backoff_then_failed(app_dsn: str, go: Go) -> None:
    go.add_shown_job()
    with psycopg.connect(app_dsn, autocommit=True) as conn:
        conn.execute("UPDATE work_items SET max_attempts = 3")
    waits: list[timedelta] = []
    r: dict[str, object] = {}
    for _ in range(3):
        with psycopg.connect(app_dsn, autocommit=True) as conn:
            conn.execute("UPDATE work_items SET run_after = now()")  # skip the wait
        item = claim_one(app_dsn)
        with psycopg.connect(app_dsn) as conn:
            assert fail(conn, item, "ValueError")
        r = row(app_dsn, item.id)
        assert isinstance(r["wait"], timedelta)
        waits.append(r["wait"])
    assert r["state"] == "failed"
    assert r["attempts"] == 3
    assert r["last_error"] == "ValueError"
    assert timedelta(seconds=25) < waits[0] < waits[1]  # 30 s, then 60 s
    with psycopg.connect(app_dsn) as conn:
        assert claim(conn, ["extract"], LEASE) is None  # failed is never claimed


def test_long_error_is_truncated(app_dsn: str, go: Go) -> None:
    go.add_shown_job()
    item = claim_one(app_dsn)
    with psycopg.connect(app_dsn) as conn:
        assert fail(conn, item, "x" * 5000)
    last_error = row(app_dsn, item.id)["last_error"]
    assert isinstance(last_error, str)
    assert len(last_error) == 500


# AC3 (design point 3): one bad item never stops the worker: the handler's exception is
# recorded and the next item is processed.
def test_process_one_records_handler_errors(
    app_dsn: str, go: Go, tmp_path: Path, caplog: pytest.LogCaptureFixture
) -> None:
    go.add_shown_job()
    go.add_shown_job()
    seen: list[int] = []

    def handler(item: Item) -> None:
        seen.append(item.id)
        if len(seen) == 1:
            raise RuntimeError("Senior Engineer at ACME: posting text must not leak")
        if len(seen) == 2:
            raise ItemError("no description")

    with open_pool(app_dsn, max_size=2) as pool:
        assert process_one(pool, {"extract": handler}, LEASE)
        assert process_one(pool, {"extract": handler}, LEASE)
        assert not process_one(pool, {"extract": handler}, LEASE)  # both now wait
    errors = [row(app_dsn, i)["last_error"] for i in seen]
    assert errors == ["RuntimeError", "ItemError: no description"]
    assert "posting text" not in caplog.text


def state_of(dsn: str, posting: int) -> str | None:
    with psycopg.connect(dsn) as conn:
        r = conn.execute(
            "SELECT state FROM work_items WHERE posting_id = %s", (posting,)
        ).fetchone()
    return r[0] if r else None


def test_process_one_completes(app_dsn: str, go: Go) -> None:
    posting = go.add_shown_job()
    with open_pool(app_dsn, max_size=2) as pool:
        assert process_one(pool, {"extract": lambda _item: None}, LEASE)
        assert not process_one(pool, {"extract": lambda _item: None}, LEASE)
    assert state_of(app_dsn, posting) == "done"


def test_no_handlers_claims_nothing(app_dsn: str, go: Go) -> None:
    posting = go.add_shown_job()
    with open_pool(app_dsn, max_size=2) as pool:
        assert not process_one(pool, {}, LEASE)
    assert state_of(app_dsn, posting) == "pending"
