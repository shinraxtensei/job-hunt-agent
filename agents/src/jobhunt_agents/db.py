"""The Postgres pool, as jobhunt_app (M3-01 AC2)."""

from psycopg_pool import ConnectionPool


def open_pool(dsn: str, *, max_size: int) -> ConnectionPool:
    # ponytail: min 1 connection, the worker does one thing at a time.
    return ConnectionPool(
        dsn,
        min_size=1,
        max_size=max_size,
        open=True,
        timeout=10,
        kwargs={"application_name": "jobhunt-worker"},
    )
