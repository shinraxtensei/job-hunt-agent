"""M3-05: the skill map and search profile: validated with line numbers, aliases resolved
ignoring case and accents, reloaded without a restart."""

import shutil
from pathlib import Path

import pytest

from jobhunt_agents.aliases import normalize
from jobhunt_agents.profile import ProfileError, ProfileWatcher, load
from tests.conftest import ROOT

PROFILE = ROOT / "data" / "profile"


@pytest.fixture
def examples(tmp_path: Path) -> Path:
    """A profile dir holding the committed examples under the real names."""
    for name in ("skill_map", "search_profile"):
        shutil.copy(PROFILE / f"{name}.example.yaml", tmp_path / f"{name}.yaml")
    return tmp_path


# AC3: the committed examples show a valid format.
def test_examples_load(examples: Path) -> None:
    p = load(examples)
    assert p.skill_map.skills
    assert p.search_profile.titles[0].rank == 1
    assert len(p.version) == 64


# The owner's real files (git-ignored) validate too, when present.
@pytest.mark.skipif(not (PROFILE / "skill_map.yaml").exists(), reason="no real profile here")
def test_real_profile_loads() -> None:
    p = load(PROFILE)
    proven = [s for s in p.skill_map.skills if s.level == "proven"]
    assert all(s.evidence for s in proven)


# AC2: names from extraction resolve to one skill through aliases, case and accents.
@pytest.mark.parametrize(
    ("extracted", "skill"),
    [
        ("Postgres", "PostgreSQL"),
        ("PostgreSQL", "PostgreSQL"),
        ("postgresql", "PostgreSQL"),
        ("POSTGRES ", "PostgreSQL"),
        ("Node.js", "Node.js"),
        ("node js", "Node.js"),
        ("NodeJS", "Node.js"),
        ("k8s", "Kubernetes"),
        ("Golang", "Go"),
        ("Intégration continue", "CI/CD"),
        ("integration continue", "CI/CD"),
        ("ci/cd", "CI/CD"),
        ("C++", "C++"),
        ("Rust", None),
        ("", None),
    ],
)
def test_alias_matching(examples: Path, extracted: str, skill: str | None) -> None:
    m = load(examples).skill_map.match(extracted)
    assert (m.name if m else None) == skill


def test_normalize() -> None:
    assert normalize("Développeur  Node.js") == normalize("developpeur node-js")
    assert normalize("C++") != normalize("C")


def write(d: Path, skill_map: str, search_profile: str) -> None:
    (d / "skill_map.yaml").write_text(skill_map)
    (d / "search_profile.yaml").write_text(search_profile)


GOOD_SEARCH = """titles:
  - en: [Backend Engineer]
    fr: [Ingénieur backend]
experience:
  professional_years: 3
  seniority: mid
"""


# AC1: every invalid line is reported, with its line number, before anything runs.
def test_every_problem_has_a_line_number(tmp_path: Path) -> None:
    write(
        tmp_path,
        """skills:
  - name: Go
    level: expert
  - name: Postgres
    level: proven
  - level: familiar
  - name: Redis
    level: familiar
    aliases: [cache, postgres]
    colour: red
""",
        """titles:
  - en: []
    fr: [Ingénieur]
experience:
  professional_years: -1
  seniority: principal
""",
    )
    with pytest.raises(ProfileError) as err:
        load(tmp_path)
    msg = str(err.value)
    for want in [
        "skill_map.yaml line 3: level",  # expert
        "skill_map.yaml line 4: proven",  # no evidence
        "skill_map.yaml line 6: name",  # missing
        "skill_map.yaml line 9: alias",  # postgres is also line 4's name
        "skill_map.yaml line 10: unknown key",
        "search_profile.yaml line 2: en",
        "search_profile.yaml line 5: professional_years",
        "search_profile.yaml line 6: seniority",
    ]:
        assert want in msg, msg
    assert len(err.value.problems) == 8


@pytest.mark.parametrize(
    ("skill_map", "want"),
    [
        ("", "skill_map.yaml line 1: want a mapping with skills"),
        ("skills: nope\n", "skill_map.yaml line 1: skills"),
        ("skills:\n  - name: [a]\n    level: learning\n", "skill_map.yaml line 2: name"),
        ("skills: [\n", "skill_map.yaml line"),  # a YAML syntax error
    ],
)
def test_malformed_files(tmp_path: Path, skill_map: str, want: str) -> None:
    write(tmp_path, skill_map, GOOD_SEARCH)
    with pytest.raises(ProfileError, match=want.replace("[", r"\[")):
        load(tmp_path)


def test_missing_file_is_named(tmp_path: Path) -> None:
    with pytest.raises(ProfileError, match=r"skill_map\.yaml"):
        load(tmp_path)


# AC4: a change is picked up without a restart (new version); a broken edit keeps the
# last good version.
def test_watcher_reloads_on_change(examples: Path) -> None:
    w = ProfileWatcher(examples)
    first = w.current()
    assert w.current() is first  # unchanged: the same object, no reload
    sm = examples / "skill_map.yaml"
    sm.write_text(sm.read_text() + "  - name: Zig\n    level: learning\n")
    second = w.current()
    assert second.version != first.version
    assert second.skill_map.match("zig") is not None
    sm.write_text("skills: [\n")
    assert w.current() is second  # broken: keep the last good one
