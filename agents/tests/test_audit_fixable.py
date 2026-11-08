"""`make vuln` (Python): fails on a vulnerability with a fix, reports unfixed ones, and
fails when the scan produced no report."""

import importlib.util
import json
from pathlib import Path
from types import ModuleType

import pytest


def tool() -> ModuleType:
    path = Path(__file__).parents[1] / "tools" / "audit_fixable.py"
    spec = importlib.util.spec_from_file_location("audit_fixable", path)
    assert spec
    assert spec.loader
    m = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(m)
    return m


def report(*fixes: list[str]) -> str:
    return json.dumps(
        {
            "vulnerabilities": [
                {"dependency": {"name": "p", "version": "1"}, "id": f"V{i}", "fix_versions": f}
                for i, f in enumerate(fixes)
            ]
        }
    )


@pytest.mark.parametrize(
    ("text", "want"),
    [
        (report(), 0),
        (report([]), 0),  # known, no fix yet: shown, not failed
        (report([], ["1.2"]), 1),  # one fixable: fail
        ("", 1),  # the scan didn't run
        ("warning: experimental", 1),
    ],
)
def test_fails_only_on_fixable(text: str, want: int) -> None:
    code, lines = tool().check(text)
    assert code == want
    assert lines
