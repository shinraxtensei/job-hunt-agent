"""The evidence bank in Postgres (M4-01 AC2, ADR-0024): written when its version changes,
in one transaction; ids missing from the file are retired, never deleted."""

from dataclasses import asdict
from typing import Any

from psycopg.types.json import Jsonb
from psycopg_pool import ConnectionPool

from jobhunt_agents.evidence import Bank, Line

_VERSION = "SELECT version FROM evidence_bank"

_ITEM = """
INSERT INTO evidence_items (id, kind, facts, position, retired, version, updated_at)
VALUES (%s, %s, %s, %s, false, %s, now())
ON CONFLICT (id) DO UPDATE SET kind = excluded.kind, facts = excluded.facts,
    position = excluded.position, retired = false, version = excluded.version, updated_at = now()
"""

_LINE = """
INSERT INTO evidence_lines (id, item_id, kind, text, variants, skills, numbers, source,
    position, retired, version, updated_at)
VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, false, %s, now())
ON CONFLICT (id) DO UPDATE SET item_id = excluded.item_id, kind = excluded.kind,
    text = excluded.text, variants = excluded.variants, skills = excluded.skills,
    numbers = excluded.numbers, source = excluded.source, position = excluded.position,
    retired = false, version = excluded.version, updated_at = now()
"""

_RETIRE_LINES = """
UPDATE evidence_lines SET retired = true, updated_at = now() WHERE version <> %s AND NOT retired
"""

_RETIRE_ITEMS = """
UPDATE evidence_items SET retired = true, updated_at = now() WHERE version <> %s AND NOT retired
"""

_BANK = """
INSERT INTO evidence_bank (id, version) VALUES (true, %s)
ON CONFLICT (id) DO UPDATE SET version = excluded.version, loaded_at = now()
"""


def _items(bank: Bank) -> list[tuple[str, str, dict[str, Any]]]:
    out: list[tuple[str, str, dict[str, Any]]] = [
        ("person", "person", bank.person.model_dump()),
        ("summary", "summary", {}),
    ]
    out += [(r.id, "role", r.model_dump(exclude={"lines"})) for r in bank.roles]
    out += [(p.id, "project", p.model_dump(exclude={"lines"})) for p in bank.projects]
    out += [(e.id, "education", e.model_dump()) for e in bank.education]
    out += [(c.id, "certification", c.model_dump()) for c in bank.certifications]
    out.append(("languages", "languages", {"languages": [x.model_dump() for x in bank.languages]}))
    return out


def _line_row(line: Line, position: int, version: str) -> tuple[Any, ...]:
    return (
        line.id,
        line.parent,
        line.kind,
        line.text,
        list(line.variants),
        Jsonb([s.model_dump() for s in line.skills]),
        Jsonb([asdict(n) for n in line.numbers]),
        line.source,
        position,
        version,
    )


def sync(pool: ConnectionPool, bank: Bank) -> bool:
    """Writes the bank if its version isn't the stored one; whether it wrote."""
    with pool.connection() as conn, conn.transaction():
        row = conn.execute(_VERSION).fetchone()
        if row and row[0] == bank.version:
            return False
        for position, (item_id, kind, facts) in enumerate(_items(bank)):
            conn.execute(_ITEM, (item_id, kind, Jsonb(facts), position, bank.version))
        for position, line in enumerate(bank.lines()):
            conn.execute(_LINE, _line_row(line, position, bank.version))
        conn.execute(_RETIRE_LINES, (bank.version,))  # what the file no longer has
        conn.execute(_RETIRE_ITEMS, (bank.version,))
        conn.execute(_BANK, (bank.version,))
    return True
