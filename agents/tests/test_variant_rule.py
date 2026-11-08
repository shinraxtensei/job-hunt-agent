"""M4-06 (owner's direction 2026-11-08): the CV variant is chosen in code, not by
decide(): `decide()` chose DIST for 173 of 509 leads. Same inputs, same answer, free."""

import pytest

from jobhunt_agents.variant import cv_variant, proxy_variant


@pytest.mark.parametrize(
    ("role_family", "title", "skills", "track", "want"),
    [
        ("backend", "Backend Engineer", ["Go", "PostgreSQL"], "relocation", "BE"),
        ("backend", "Backend Engineer, Payments Experiences", ["Java"], "relocation", "PAY"),
        ("backend", "Software Engineer", ["Payments", "KYC"], "remote", "PAY"),  # by skills
        ("fullstack", "Full Stack Engineer, Revenue", ["React"], "remote", "FS"),
        ("frontend", "Senior UI Engineer", ["React", "CSS"], "relocation", "FS"),
        ("mobile", "Senior Android Engineer", ["Kotlin"], "relocation", "FS"),
        ("ai_ml", "Senior Applied Scientist", ["Python"], "relocation", "AI"),
        ("backend", "LLM Platform Engineer", ["Python"], "remote", "AI"),  # title says AI
        ("platform", "Site Reliability Engineer", ["Kubernetes"], "remote", "DIST"),
        ("data", "Software Engineer - Data Infrastructure - Kafka", ["Kafka"], "remote", "DIST"),
        ("backend", "Distributed Systems Engineer", ["Go"], "remote", "DIST"),
        # "Platform" as a team name: a backend title stays BE
        ("backend", "Senior Backend Engineer - Content Platform", ["Java"], "relocation", "BE"),
        ("unknown", "Senior Customer Engineer, Majors", ["DNS"], "relocation", "CONS"),
        ("backend", "Backend Engineer", ["Java"], "consulting", "CONS"),  # consulting company
        ("unknown", "Software Engineer", [], "remote", "BE"),
    ],
)
def test_cv_variant(role_family: str, title: str, skills: list[str], track: str, want: str) -> None:
    assert cv_variant(role_family, title, skills, track) == want


# The proxy for the owner's labels until OI-3: the role family alone.
@pytest.mark.parametrize(
    ("role_family", "want"),
    [
        ("backend", "BE"),
        ("fullstack", "FS"),
        ("frontend", "FS"),
        ("mobile", "FS"),
        ("platform", "DIST"),
        ("data", "DIST"),
        ("ai_ml", "AI"),
        ("customer", "CONS"),
        ("unknown", "BE"),
    ],
)
def test_proxy_variant(role_family: str, want: str) -> None:
    assert proxy_variant(role_family) == want
