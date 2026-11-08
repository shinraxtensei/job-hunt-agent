"""`make vuln` for Python (M5-03, ADR-0027): reads `uv audit --output-format json` on stdin,
lists every known vulnerability, and fails when one has a fix available (as govulncheck
does for Go). Unfixed ones are shown, not failed on: there is nothing to upgrade to.
No output, or output that isn't the audit's JSON, fails too (a scan that didn't run)."""

import json
import sys
from typing import Any, cast


def check(text: str) -> tuple[int, list[str]]:
    try:
        report = json.loads(text)
        vulns = cast(list[dict[str, Any]], report["vulnerabilities"])
    except ValueError, KeyError, TypeError:
        return 1, ["uv audit produced no report"]
    lines: list[str] = []
    fixable = 0
    for v in vulns:
        dep = v["dependency"]
        fixes = [str(f) for f in cast(list[Any], v.get("fix_versions") or [])]
        fixable += bool(fixes)
        fix = f"fixed in {', '.join(fixes)}" if fixes else "no fix yet"
        lines.append(f"{dep['name']} {dep['version']}: {v['id']} ({fix}) {v.get('summary', '')}")
    lines.append(f"python: {len(vulns)} known vulnerabilities, {fixable} with a fix")
    return (1 if fixable else 0), lines


if __name__ == "__main__":
    code, out = check(sys.stdin.read())
    print("\n".join(out))
    sys.exit(code)
