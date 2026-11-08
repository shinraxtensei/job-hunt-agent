"""M4-01: the evidence bank file, validated with line numbers (ADR-0024), and the numbers
each line contains."""

import shutil
from pathlib import Path

import pytest

from jobhunt_agents.evidence import EvidenceError, EvidenceWatcher, load_evidence
from jobhunt_agents.numbers import Number, numbers
from jobhunt_agents.profile import Profile, load
from tests.conftest import ROOT

EXAMPLE = ROOT / "data" / "profile" / "evidence.example.yaml"


@pytest.fixture
def profile(tmp_path: Path) -> Profile:
    d = tmp_path / "profile"
    d.mkdir()
    for name in ("skill_map", "search_profile"):
        shutil.copy(ROOT / "data" / "profile" / f"{name}.example.yaml", d / f"{name}.yaml")
    return load(d)


def write(tmp_path: Path, text: str) -> Path:
    p = tmp_path / "evidence.yaml"
    p.write_text(text)
    return p


# AC1, AC4: the committed example is valid (placeholder data, the example skill map).
def test_example_loads(profile: Profile) -> None:
    bank = load_evidence(EXAMPLE, profile)
    assert bank.person.name == "Sam Example"
    assert [r.id for r in bank.roles] == ["example-corp"]
    assert {line.id for line in bank.lines()} == {
        "summary-years",
        "example-billing",
        "example-deploys",
        "side-project-collector",
    }
    assert len(bank.version) == 64


# AC1: each rule, one broken file, the problem names its line.
@pytest.mark.parametrize(
    ("old", "new", "message"),
    [
        ("id: example-deploys", "id: example-billing", "id 'example-billing' is also used"),
        ("skills: [Docker, CI/CD]", "skills: [Docker, Cobol]", "'Cobol' is not in your skill map"),
        ("start: 2023-05", "start: May 2023", "want YYYY-MM, YYYY or present"),
        (
            'text: "Moved the services to Docker images built in CI."\n        checked: true',
            'text: "Moved the services to Docker images built in CI."\n        checked: false',
            "not checked",
        ),
        ("variants: [BE, DIST]", "variants: [BE, OPS]", "variant 'OPS'"),
        (
            "skills: [Docker, CI/CD]",
            "skills: [Docker, Go]",
            "learning skill 'Go' is allowed only in a project line",
        ),
        (
            'location: "Casablanca"',
            'location: "Casablanca"\n    colour: blue',
            "unknown key 'colour'",
        ),
        ("id: example-corp", "id: summary  # reserved", "id 'summary' is reserved"),
    ],
)
def test_each_rule_names_its_line(
    tmp_path: Path, profile: Profile, old: str, new: str, message: str
) -> None:
    text = EXAMPLE.read_text()
    assert old in text
    broken = text.replace(old, new, 1)
    with pytest.raises(EvidenceError) as err:
        load_evidence(write(tmp_path, broken), profile)
    # the line of the replacement's last line, where it was put (not a comment elsewhere)
    want_line = broken[: broken.index(new) + len(new)].count("\n") + 1
    assert any(message in p and f"line {want_line}" in p for p in err.value.problems), (
        err.value.problems
    )


# Learning skills: a project line may name one (stated as fact); a summary may not.
def test_learning_only_in_project_lines(tmp_path: Path, profile: Profile) -> None:
    text = EXAMPLE.read_text()
    bad = text.replace(
        'skills: [Node.js, PostgreSQL]\n    text: "Backend', 'skills: [Go]\n    text: "Backend', 1
    )
    assert bad != text
    with pytest.raises(EvidenceError, match="allowed only in a project line"):
        load_evidence(write(tmp_path, bad), profile)


# AC3: each line records its skills with their levels and its numbers.
def test_lines_record_levels_and_numbers(profile: Profile) -> None:
    bank = load_evidence(EXAMPLE, profile)
    billing = bank.line("example-billing")
    assert [(s.skill, s.level) for s in billing.skills] == [
        ("Node.js", "proven"),
        ("PostgreSQL", "proven"),
    ]
    assert billing.numbers == (Number(40, ""), Number(6, "minute"))


# A broken edit keeps the last good version (as the profile does).
def test_watcher_keeps_last_good(tmp_path: Path, profile: Profile) -> None:
    p = write(tmp_path, EXAMPLE.read_text())
    w = EvidenceWatcher(p)
    first = w.current(profile)
    assert first is not None
    p.write_text(p.read_text().replace("id: example-deploys", "id: example-billing"))  # broken
    assert w.current(profile) is first
    p.write_text(EXAMPLE.read_text().replace("6 minutes", "5 minutes"))
    again = w.current(profile)
    assert again is not None
    assert again.version != first.version


def test_missing_file_is_none(tmp_path: Path, profile: Profile) -> None:
    assert EvidenceWatcher(tmp_path / "evidence.yaml").current(profile) is None


# AC3 and M4-04 AC5: numbers, normalised; digits inside names are not numbers.
@pytest.mark.parametrize(
    ("text", "want"),
    [
        ("cut heap usage by ~53% in production", [Number(53, "%", approx=True)]),
        ("~3 years of production experience", [Number(3, "year", approx=True)]),
        ("5+ years", [Number(5, "year", plus=True)]),
        ("from 40 to 6 minutes", [Number(40, ""), Number(6, "minute")]),
        ("40 percent", [Number(40, "%")]),
        ("1.2k requests", [Number(1200, "")]),
        ("~227k LOC", [Number(227000, "", approx=True)]),
        ("40,000 orders", [Number(40000, "")]),
        ("24h TTL", [Number(24, "hour")]),
        ("500-doc streaming bulk ops", [Number(500, "")]),
        ("Resolved 5 production memory leaks", [Number(5, "")]),
        # names, not numbers
        ("OAuth2 and AWS S3 with GPT-4 on K8s, 3DS cards, Web3", []),
        ("Python 3 and Java 21 services", []),
        ("partitioning, PgBouncer, N+1 fixes", []),  # a pattern's name
        ("cut Python 3 build time by 40%", [Number(40, "%")]),
    ],
)
def test_numbers(text: str, want: list[Number]) -> None:
    assert numbers(text, names=("Python", "Java")) == want


# A broken file is reported once per version, not on every worker iteration.
def test_broken_file_warns_once(
    tmp_path: Path, profile: Profile, caplog: pytest.LogCaptureFixture
) -> None:
    p = write(tmp_path, EXAMPLE.read_text().replace("id: example-deploys", "id: example-billing"))
    w = EvidenceWatcher(p)
    for _ in range(3):
        assert w.current(profile) is None
    assert sum("evidence not loaded" in r.message for r in caplog.records) == 1
