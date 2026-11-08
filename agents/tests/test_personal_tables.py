"""M6-03 AC3 (OI-14): notes and contacts never reach a model. The worker is the only code
that calls one, so no SQL in the agents may read those tables."""

import re
from pathlib import Path

SRC = Path(__file__).parents[1] / "src"
SQL = re.compile(r"\b(FROM|JOIN|INTO|UPDATE)\s+(notes|contacts)\b", re.IGNORECASE)


def test_agents_never_read_notes_or_contacts() -> None:
    found = [
        f"{p.relative_to(SRC)}: {m.group(0)}"
        for p in sorted(SRC.rglob("*.py"))
        for m in SQL.finditer(p.read_text())
    ]
    assert found == []


def test_the_rule_bites() -> None:
    assert SQL.search("SELECT body FROM notes WHERE id = %s")
    assert SQL.search("x JOIN contacts c ON c.application_id = a.id")
    assert not SQL.search("the release notes from the board")
