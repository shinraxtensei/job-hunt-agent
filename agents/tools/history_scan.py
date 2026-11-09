"""Scan every blob in the git history for the owner's personal data (M8-03, before the
repository goes public). gitleaks finds secret-shaped strings; this finds *our* values:

- the evidence bank's person block (name, email, phone, links), every line's text and
  text_fr, employer, school and project names;
- every value in .env (secrets and passwords);
- the latest real fit reasons from the database (with --db and DATABASE_URL);
- files committed under the personal paths (git-ignored now, maybe not always).

Values are loaded at run time and never printed: a finding names the value's label
(".env:KEY", "evidence:line-id"), the path and the commits. Images are listed for a
person to look at; text inside them can't be searched.

Run: cd agents && uv run python tools/history_scan.py [--db]
"""

import os
import re
import shutil
import subprocess
import sys
from collections import defaultdict
from pathlib import Path
from typing import Any, cast

import psycopg
from ruamel.yaml import YAML

ROOT = Path(__file__).resolve().parents[2]
GIT = shutil.which("git") or "/usr/bin/git"
PERSONAL_PATHS = re.compile(
    r"^(data/profile/(?!.*\.example\.yaml$).+|agents/evals/(postings|private)/.+"
    r"|docs/design/m7/private/.+|\.env(\..+)?|.*\.dump|.*personal\.tar\.gz)$"
)
IMAGE = re.compile(r"\.(png|jpe?g|gif|webp|pdf)$", re.IGNORECASE)
MIN_LEN = 6
SECRET_KEY = re.compile(r"KEY|PASSWORD|SECRET|TOKEN|URL|DSN", re.IGNORECASE)  # not slugs or ports
SENTENCE = 25  # evidence text shorter than this is too common to mean anything


def git(*args: str) -> bytes:
    return subprocess.run([GIT, *args], cwd=ROOT, check=True, capture_output=True).stdout


def _lines(items: object, out: dict[str, str]) -> None:
    for x in cast(list[dict[str, Any]], items or []):
        for k in ("text", "text_fr"):
            v = x.get(k)
            if isinstance(v, str) and len(v) >= SENTENCE:
                out[f"evidence:{x.get('id')}.{k}"] = v


def evidence_values() -> dict[str, str]:
    ev = ROOT / "data/profile/evidence.yaml"
    if not ev.is_file():
        return {}
    doc = cast(dict[str, Any], YAML(typ="safe").load(ev.read_text()) or {})  # pyright: ignore[reportUnknownMemberType]
    person = cast(dict[str, Any], doc.get("person") or {})
    out = {f"person.{k}": str(person[k]) for k in ("name", "email", "phone") if person.get(k)}
    links = cast(list[object], person.get("links") or [])
    out |= {f"person.links[{i}]": str(x) for i, x in enumerate(links)}
    if person.get("phone"):
        out["person.phone(digits)"] = re.sub(r"\D", "", str(person["phone"]))[-9:]
    _lines(doc.get("summary"), out)
    _lines(doc.get("letter"), out)
    for kind, name_key in (("roles", "employer"), ("projects", "name"), ("education", "school")):
        for item in cast(list[dict[str, Any]], doc.get(kind) or []):
            out[f"{kind}:{item.get('id')}"] = str(item.get(name_key, ""))
            _lines(item.get("lines"), out)
    return out


def env_values() -> dict[str, str]:
    env = ROOT / ".env"
    out: dict[str, str] = {}
    if env.is_file():
        for line in env.read_text().splitlines():
            if "=" in line and not line.lstrip().startswith("#"):
                k, v = line.split("=", 1)
                if SECRET_KEY.search(k):
                    out[f".env:{k.strip()}"] = v.strip().strip("'\"")
    return out


def reason_values() -> dict[str, str]:
    dsn = os.environ.get("DATABASE_URL")
    if "--db" not in sys.argv or not dsn:
        return {}
    with psycopg.connect(dsn) as conn:
        rows = conn.execute(
            "SELECT posting_id, reason FROM score_reasons ORDER BY created_at DESC LIMIT 300"
        ).fetchall()
    return {f"reason:posting {pid}": str(reason) for pid, reason in rows}


def needles() -> dict[str, str]:
    """label -> value, lower-cased; never printed."""
    found = evidence_values() | env_values() | reason_values()
    return {k: v.lower() for k, v in found.items() if len(v) >= MIN_LEN}


def history() -> tuple[dict[str, set[str]], dict[str, set[str]]]:
    """Every object's paths, and the commits that touched each path."""
    paths: dict[str, set[str]] = defaultdict(set)
    for line in git("rev-list", "--all", "--objects").decode().splitlines():
        sha, _, path = line.partition(" ")
        if path:
            paths[sha].add(path)
    commits: dict[str, set[str]] = defaultdict(set)
    for block in git("log", "--all", "--format=@%h", "--name-only").decode().split("@")[1:]:
        c, *files = block.split("\n")
        for f in filter(None, files):
            commits[f].add(c)
    return paths, commits


def scan(
    find: dict[str, str], paths: dict[str, set[str]], commits: dict[str, set[str]]
) -> dict[tuple[str, str], set[str]]:
    """(label, path) -> commits, for every text blob containing a value."""
    hits: dict[tuple[str, str], set[str]] = defaultdict(set)
    for sha, ps in paths.items():
        if any(IMAGE.search(p) for p in ps) or git("cat-file", "-t", sha).strip() != b"blob":
            continue
        text = git("cat-file", "-p", sha).decode("utf-8", "replace").lower()
        for label in (label for label, value in find.items() if value in text):
            for p in ps:
                hits[(label, p)] |= commits.get(p, set())
    return hits


def show(title: str, rows: list[str]) -> None:
    print(f"\n== {title} ({len(rows)})")
    for r in rows:
        print(r)


def main() -> int:
    find = needles()
    print(f"{len(find)} personal values loaded (not shown)")
    paths, commits = history()
    hits = scan(find, paths, commits)
    every = sorted({p for ps in paths.values() for p in ps})

    def when(p: str) -> str:
        return " ".join(sorted(commits.get(p, set()))[:6])

    show(
        "Personal values found in text",
        [
            f"{label}\t{p}\t{len(cs)} commits: {' '.join(sorted(cs)[:6])}"
            for (label, p), cs in sorted(hits.items())
        ],
    )
    show(
        "Files under personal paths ever committed",
        [f"{p}\t{when(p)}" for p in every if PERSONAL_PATHS.match(p)],
    )
    show("Images to look at", [f"{p}\t{when(p)}" for p in every if IMAGE.search(p)])
    return 0


if __name__ == "__main__":
    sys.exit(main())
