"""M4-04: the draft validator and the six guardrail tests (evidence-bank-guardrails). A
draft that adds, inflates or alters anything is blocked, whatever the model wrote. Fixed
drafts against the committed example bank; no model.

Example bank: summary-years ("Backend engineer with 3 years building Node.js services on
PostgreSQL"); role example-corp, "Backend Engineer" at Example Corp from 2023-05, lines
example-billing ("…cutting its run time from 40 to 6 minutes", Node.js + PostgreSQL) and
example-deploys (Docker + CI/CD, Docker familiar); project side-project, line
side-project-collector ("Built a job collector in Go…", Go learning).
"""

import shutil

import pytest

from jobhunt_agents.coverage import coverage
from jobhunt_agents.drafts import Bullet, DraftContent, Section
from jobhunt_agents.evals.data import INJECTIONS
from jobhunt_agents.evidence import Bank, load_evidence
from jobhunt_agents.extraction import Extraction
from jobhunt_agents.profile import Profile, load
from jobhunt_agents.validator import validate
from tests.conftest import ROOT

EXAMPLE = ROOT / "data" / "profile" / "evidence.example.yaml"
BILLING = "Rebuilt the billing service's invoice job, cutting its run time from 40 to 6 minutes."


@pytest.fixture(scope="module")
def profile(tmp_path_factory: pytest.TempPathFactory) -> Profile:
    d = tmp_path_factory.mktemp("profile")
    for name in ("skill_map", "search_profile"):
        shutil.copy(ROOT / "data" / "profile" / f"{name}.example.yaml", d / f"{name}.yaml")
    return load(d)


@pytest.fixture(scope="module")
def bank(profile: Profile) -> Bank:
    return load_evidence(EXAMPLE, profile)


def draft(
    role: list[Bullet] | None = None,
    project: list[Bullet] | None = None,
    summary: list[Bullet] | None = None,
    skills: list[str] | None = None,
) -> DraftContent:
    return DraftContent(
        summary=summary or [],
        roles=[Section(id="example-corp", bullets=role or [])],
        projects=[Section(id="side-project", bullets=project or [])],
        skills=skills or [],
    )


def b(text: str, *ids: str) -> Bullet:
    return Bullet(text=text, evidence_ids=list(ids))


def rules(d: DraftContent, profile: Profile, bank: Bank) -> list[str]:
    return [f.rule for f in validate(d, bank, profile).violations]


def ok(d: DraftContent, profile: Profile, bank: Bank) -> None:
    r = validate(d, bank, profile)
    assert r.violations == [], r.violations
    assert r.status == "ready_for_review"


# A faithful rephrase passes.
def test_faithful_draft_passes(profile: Profile, bank: Bank) -> None:
    ok(
        draft(
            role=[
                b(
                    "Cut the invoice job's run time from 40 to 6 minutes on Node.js, PostgreSQL.",
                    "example-billing",
                )
            ],
            project=[
                b(
                    "Built a job collector in Go, storing postings in PostgreSQL.",
                    "side-project-collector",
                )
            ],
            summary=[
                b("Backend engineer with 3 years building Node.js services.", "summary-years")
            ],
            skills=["PostgreSQL", "Node.js", "Docker"],
        ),
        profile,
        bank,
    )


# Guardrail test 6 ("every bullet has evidence IDs that exist in the bank") and AC2.
@pytest.mark.parametrize(
    "ids",
    [(), ("no-such-line",), ("side-project-collector",)],  # none, unknown, another section's
)
def test_bullets_need_their_own_existing_evidence(
    profile: Profile, bank: Bank, ids: tuple[str, ...]
) -> None:
    assert rules(draft(role=[b(BILLING, *ids)]), profile, bank) == ["unsupported"]


def test_retired_line_is_unsupported(profile: Profile, bank: Bank) -> None:
    r = validate(
        draft(role=[b(BILLING, "example-billing")]), bank, profile, retired={"example-billing"}
    )
    assert [f.rule for f in r.violations] == ["unsupported"]


def extraction(*required: str) -> Extraction:
    return Extraction.model_validate(
        {
            "required_skills": list(required),
            "optional_skills": [],
            "min_years": None,
            "seniority": "unknown",
            "visa_sponsorship": "unknown",
            "salary": None,
            "languages": [],
            "remote_policy": "unknown",
        }
    )


# Guardrail test 1 (planted skill) and AC3: the posting requires Rust, absent from the
# evidence bank: coverage lists it as a gap, and a draft that names it is blocked.
def test_planted_skill_is_unsupported(profile: Profile, bank: Bank) -> None:
    cov = coverage(extraction("PostgreSQL", "Rust"), profile, bank, "BE")
    assert next(s for s in cov.skills if s.name == "Rust").status == "gap"
    d = draft(role=[b("Cut the invoice job from 40 to 6 minutes with Rust.", "example-billing")])
    r = validate(d, bank, profile, coverage=cov)
    assert [f.rule for f in r.violations] == ["unsupported"]
    assert "Rust" in r.violations[0].detail


# Guardrail test 3 (altered fact) and AC4: a title, employer or year not in the evidence.
@pytest.mark.parametrize(
    "text",
    [
        "As Senior Software Engineer, cut the invoice job from 40 to 6 minutes.",
        "In 2022, cut the invoice job from 40 to 6 minutes.",
    ],
)
def test_altered_fact(profile: Profile, bank: Bank, text: str) -> None:
    # (an unknown company name can't be told from words in code: M5-02's checker covers it)
    assert rules(draft(role=[b(text, "example-billing")]), profile, bank) == ["altered_fact"]


def test_own_title_and_year_pass(profile: Profile, bank: Bank) -> None:
    ok(
        draft(
            role=[
                b(
                    "As Backend Engineer since 2023, cut the invoice job from 40 to 6 minutes.",
                    "example-billing",
                )
            ]
        ),
        profile,
        bank,
    )


def test_other_employer_is_altered(profile: Profile, bank: Bank) -> None:
    # a project bullet claiming the employer
    d = draft(
        project=[b("Built a job collector in Go for Example Corp.", "side-project-collector")]
    )
    assert rules(d, profile, bank) == ["altered_fact"]


# Guardrail test 4 (inflated number) and AC5.
@pytest.mark.parametrize(
    ("text", "ids", "want"),
    [
        ("Cut the invoice job from 40 to 5 minutes.", ("example-billing",), ["inflated_number"]),
        ("Cut the invoice job by 50%.", ("example-billing",), ["inflated_number"]),
        ("Backend engineer with 5+ years on Node.js.", ("summary-years",), ["inflated_number"]),
        (
            "Backend engineer with 3+ years on Node.js.",
            ("summary-years",),
            ["inflated_number"],
        ),  # "+" added
        (
            "Cut the invoice job from forty to 6 minutes.",
            ("example-billing",),
            [],
        ),  # 40 not restated
        (
            "Cut the invoice job's run time from 40 to 6 minutes using PostgreSQL 16.",
            ("example-billing",),
            [],
        ),  # a version
    ],
)
def test_inflated_number(
    profile: Profile, bank: Bank, text: str, ids: tuple[str, ...], want: list[str]
) -> None:
    where = "summary" if ids == ("summary-years",) else "role"
    d = draft(summary=[b(text, *ids)]) if where == "summary" else draft(role=[b(text, *ids)])
    assert rules(d, profile, bank) == want


# Guardrail test 5 (learning skill as experience) and AC6: skills a bullet names.
def test_learning_skill_only_in_its_project_bullet(profile: Profile, bank: Bank) -> None:
    ok(draft(project=[b("Built a job collector in Go.", "side-project-collector")]), profile, bank)
    # under a role, citing a role line that doesn't name Go
    assert rules(
        draft(role=[b("Rebuilt the invoice job in Go.", "example-billing")]), profile, bank
    ) == ["unsupported"]
    # in the summary, even citing the project line
    s = draft(summary=[b("Builds job collectors in Go.", "side-project-collector")])
    assert rules(s, profile, bank) == ["learning_as_experience"]
    # in the skills section
    assert rules(draft(skills=["PostgreSQL", "Go"]), profile, bank) == ["learning_as_experience"]


def test_familiar_skill_in_a_bullet(profile: Profile, bank: Bank) -> None:
    # Docker is familiar: fine where a cited line names it, blocked otherwise
    ok(
        draft(role=[b("Moved the services to Docker images built in CI.", "example-deploys")]),
        profile,
        bank,
    )
    d = draft(
        role=[b("Rebuilt the invoice job in Docker, from 40 to 6 minutes.", "example-billing")]
    )
    assert rules(d, profile, bank) == ["familiar_in_bullet"]


def test_skills_section_lists_only_known_skills(profile: Profile, bank: Bank) -> None:
    assert rules(draft(skills=["PostgreSQL", "Cobol"]), profile, bank) == ["unsupported"]


# AC7: scope inflation is a warning, not a block; EN and FR.
@pytest.mark.parametrize(
    ("text", "warned"),
    [
        ("Led the rebuild of the invoice job, from 40 to 6 minutes.", True),
        ("Architected the invoice job, from 40 to 6 minutes.", True),
        ("Managed the invoice job rebuild, from 40 to 6 minutes.", True),
        ("A dirigé la refonte du job de facturation, de 40 à 6 minutes.", True),
        ("Rebuilt the invoice job, from 40 to 6 minutes.", False),
    ],
)
def test_scope_inflation_warns(profile: Profile, bank: Bank, text: str, warned: bool) -> None:
    r = validate(draft(role=[b(text, "example-billing")]), bank, profile)
    assert r.violations == []
    assert r.status == "ready_for_review"
    assert [w.rule for w in r.warnings] == (["scope_inflation"] if warned else [])


# Guardrail test 2 (prompt injection), AC8: an injected "skill" in the extraction is a
# suspicious gap, flagged on the draft; it never becomes a claim.
@pytest.mark.parametrize("injection", [i.text for i in INJECTIONS])
def test_injection_is_flagged(profile: Profile, bank: Bank, injection: str) -> None:
    cov = coverage(extraction("PostgreSQL", injection), profile, bank, "BE")
    r = validate(draft(role=[b(BILLING, "example-billing")]), bank, profile, coverage=cov)
    assert r.violations == []
    assert [w.rule for w in r.warnings] == ["suspicious_input"]


def test_findings_carry_sentence_and_closest_line(profile: Profile, bank: Bank) -> None:
    r = validate(
        draft(role=[b("Cut the invoice job from 40 to 5 minutes.", "example-billing")]),
        bank,
        profile,
    )
    f = r.violations[0]
    assert f.sentence == "Cut the invoice job from 40 to 5 minutes."
    assert f.closest == "example-billing"
    assert f.where == "roles[example-corp].bullets[0]"
    assert r.status == "blocked"
