"""M3-04 review: the committed tech-alias table (not personal), used by the eval metric
and the M3-05 skill matcher, with the skill map's own aliases on top."""

from jobhunt_agents.aliases import TABLE, canon
from jobhunt_agents.profile import _Problems, _skill_map  # pyright: ignore[reportPrivateUsage]


def test_aliases_resolve_to_one_canonical_name() -> None:
    assert canon("GCP") == canon("Google Cloud Platform")
    assert canon("k8s") == canon("Kubernetes")
    assert canon("Postgres") == canon("postgresql")
    assert canon("Payment systems") == canon("Payments")
    assert canon("Rust") == "rust"  # not in the table: folded only
    assert canon("Go") != canon("Rust")


def test_table_has_no_alias_under_two_names() -> None:
    seen: dict[str, str] = {}
    for name, aliases in TABLE.items():
        for a in (name, *aliases):
            assert seen.setdefault(canon(a), name) == name, a


def test_skill_map_matches_through_the_table() -> None:
    problems = _Problems("skill_map.yaml")
    sm = _skill_map("skills:\n- name: Google Cloud Platform\n  level: familiar\n", problems)
    assert sm is not None
    m = sm.match("GCP")
    assert m is not None
    assert m.name == "Google Cloud Platform"


def test_two_skills_that_are_one_tech_collide() -> None:
    problems = _Problems("skill_map.yaml")
    _skill_map(
        "skills:\n- name: PostgreSQL\n  level: familiar\n- name: Postgres\n  level: familiar\n",
        problems,
    )
    assert problems.items
