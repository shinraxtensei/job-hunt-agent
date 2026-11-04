"""The work queue's consumer side (ADR-0001, M3-02). Go enqueues; this claims.

Claim takes a lease in one short statement and commits; the work runs outside any
transaction; complete and fail are fenced by the claim token, so a worker whose lease
expired and whose item was claimed again cannot acknowledge it.
"""

import logging
import uuid
from collections.abc import Callable, Mapping, Sequence
from dataclasses import dataclass
from datetime import timedelta

import psycopg
from psycopg_pool import ConnectionPool

log = logging.getLogger(__name__)

MAX_ERROR = 500  # the column's CHECK
BACKOFF_BASE = timedelta(seconds=30)
BACKOFF_CAP = timedelta(hours=1)


@dataclass(frozen=True)
class Item:
    """A claimed work item: ids only, never posting text."""

    id: int
    posting_id: int
    kind: str
    content_hash: bytes
    attempts: int  # including this claim
    claim_token: uuid.UUID


Handler = Callable[[Item], None]


class ItemError(Exception):
    """A per-item failure whose message is safe to store: no posting text, no model output.
    Any other exception is stored as its class name only."""


class NeedsReview(ItemError):
    """Retrying won't help (the model client already retried): the item fails at once and
    waits for the owner (ADR-0001's needs_review)."""


_CLAIM = """
WITH expired AS (  -- a lease that ran out on the last attempt: give up on it
    UPDATE work_items
    SET state = 'failed', locked_until = NULL, claim_token = NULL, updated_at = now(),
        last_error = 'lease expired on the last attempt'
    WHERE state = 'running' AND locked_until < now() AND attempts >= max_attempts
)
UPDATE work_items
SET state = 'running', locked_until = now() + %(lease)s, attempts = attempts + 1,
    claim_token = gen_random_uuid(), updated_at = now()
WHERE id = (
    SELECT id FROM work_items
    WHERE kind = ANY(%(kinds)s) AND attempts < max_attempts
      AND ((state = 'pending' AND run_after <= now())
           OR (state = 'running' AND locked_until < now()))
    ORDER BY run_after, id
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
RETURNING id, posting_id, kind, content_hash, attempts, claim_token
"""

_COMPLETE = """
UPDATE work_items
SET state = 'done', locked_until = NULL, claim_token = NULL, last_error = NULL, updated_at = now()
WHERE id = %(id)s AND claim_token = %(token)s
"""

_FAIL = """
UPDATE work_items
SET state = CASE WHEN %(final)s OR attempts >= max_attempts THEN 'failed' ELSE 'pending' END,
    run_after = now() + least(%(cap)s, %(base)s * power(2, attempts - 1)),
    last_error = left(%(error)s, %(max_error)s),
    locked_until = NULL, claim_token = NULL, updated_at = now()
WHERE id = %(id)s AND claim_token = %(token)s
"""


def claim(conn: psycopg.Connection, kinds: Sequence[str], lease: timedelta) -> Item | None:
    """Claims the next runnable item of these kinds, or None. Commits before returning."""
    with conn.transaction():
        row = conn.execute(_CLAIM, {"lease": lease, "kinds": list(kinds)}).fetchone()
    return Item(*row) if row else None


def complete(conn: psycopg.Connection, item: Item) -> bool:
    """Marks the item done; False if this claim no longer owns it."""
    with conn.transaction():
        return conn.execute(_COMPLETE, {"id": item.id, "token": item.claim_token}).rowcount == 1


def fail(conn: psycopg.Connection, item: Item, error: str, *, final: bool = False) -> bool:
    """Schedules a retry with backoff, or marks the item failed on its last attempt (or at
    once if final); False if this claim no longer owns it. `error` must hold no posting
    text."""
    params = {
        "final": final,
        "id": item.id,
        "token": item.claim_token,
        "error": error,
        "max_error": MAX_ERROR,
        "base": BACKOFF_BASE,
        "cap": BACKOFF_CAP,
    }
    with conn.transaction():
        return conn.execute(_FAIL, params).rowcount == 1


def safe_error(e: Exception) -> str:
    if isinstance(e, ItemError):
        return f"{type(e).__name__}: {e}"[:MAX_ERROR]
    return type(e).__name__  # its message could quote posting text


def process_one(pool: ConnectionPool, handlers: Mapping[str, Handler], lease: timedelta) -> bool:
    """Claims one item and runs its handler; True if there was an item.

    A handler error is recorded on the item, never raised: one bad item never stops the
    worker. Database errors are raised (the worker decides when they are fatal)."""
    with pool.connection() as conn:
        item = claim(conn, list(handlers), lease)
    if item is None:
        return False
    fields = {"item_id": item.id, "kind": item.kind, "attempt": item.attempts}
    try:
        handlers[item.kind](item)
    except psycopg.OperationalError:
        raise
    except Exception as e:  # noqa: BLE001 - recorded on the item; the worker carries on
        err = safe_error(e)
        log.warning("item failed", extra={**fields, "error": err})
        with pool.connection() as conn:
            owned = fail(conn, item, err, final=isinstance(e, NeedsReview))
    else:
        with pool.connection() as conn:
            owned = complete(conn, item)
        if owned:
            log.info("item done", extra=fields)
    if not owned:
        log.warning("acknowledgement rejected: the lease was lost", extra=fields)
    return True
