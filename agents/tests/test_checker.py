"""M5-02: the claim checker. An independent model pass over every sentence (CV and
letter), with the cited lines and the bank's facts and never the posting; a verdict other
than supported blocks the draft; verdicts that don't match the sentences are malformed.
Fake provider: CI never calls the real API."""

import json
from decimal import Decimal
from pathlib import Path
from typing import Any

import pytest

from jobhunt_agents.checker import Checker, Sentence, sentences
from jobhunt_agents.db import open_pool
from jobhunt_agents.evidence import Bank
from jobhunt_agents.model import InvalidOutput, ModelClient
from tests.conftest import Go
from tests.fakes import FakeProvider, answer
from tests.test_tailor import (
    GOOD,
    LETTER,
    latest,
    lead,
    run,
    verdicts,
)

ITEMS = [
    Sentence(
        "summary[0]", "Backend engineer with 3 years building Node.js services.", ("summary-years",)
    ),
    Sentence("letter[0][0]", "I am available to start within one month.", ("letter-availability",)),
]


def checker(provider: FakeProvider, dsn: str) -> tuple[Checker, Any]:
    pool = open_pool(dsn, max_size=1)
    return Checker(ModelClient(provider=provider, pool=pool, daily_budget_usd=Decimal("1"))), pool


# Sentences in reading order, named as the validator names them (the page puts each
# finding under its sentence).
def test_sentences_in_reading_order() -> None:
    letter = {"paragraphs": [[b for p in LETTER["paragraphs"] for b in p["sentences"]]]}
    got = [s.where for s in sentences(GOOD, letter)]
    assert got == ["summary[0]", "roles[example-corp].bullets[0]", "letter[0][0]", "letter[0][1]"]


# AC1: the checker reads the sentences, their cited lines and the bank's facts; nothing
# from the posting, and not the tailoring prompt.
def test_checker_sees_no_posting(app_dsn: str, go: Go, profile_dir: Path) -> None:
    posting = lead(app_dsn, go, profile_dir)
    provider = FakeProvider(answer(json.dumps(GOOD)), answer(json.dumps(LETTER)), verdicts(4))
    run(app_dsn, profile_dir, provider)
    assert latest(app_dsn, posting)["status"] == "ready_for_review"
    req = provider.requests[2]
    assert "strict" in req.system  # the checker's own prompt
    assert "Cut the invoice job's run time from 40 to 6 minutes on Node.js." in req.user
    assert "cutting its run time from 40 to 6 minutes" in req.user  # the cited line
    assert "Backend Engineer at Example Corp" in req.user  # the facts
    for posting_only in ("Kubernetes", "Rust", "Job skills", "covered", "Words: at most"):
        assert posting_only not in req.user


# AC3: an unsupported or altered verdict blocks the draft, as a finding under its sentence.
def test_unsupported_verdict_blocks(app_dsn: str, go: Go, profile_dir: Path) -> None:
    posting = lead(app_dsn, go, profile_dir)
    provider = FakeProvider(
        answer(json.dumps(GOOD)),
        answer(json.dumps(LETTER)),
        verdicts(4, {1: "altered_fact", 3: "unsupported"}),
        verdicts(4, {1: "altered_fact", 3: "unsupported"}),  # M9-06: the second run agrees
    )
    run(app_dsn, profile_dir, provider)
    d = latest(app_dsn, posting)
    assert d["status"] == "blocked"
    got = [(f["rule"], f["where"]) for f in d["findings"]["violations"]]
    assert got == [
        ("checker_altered_fact", "roles[example-corp].bullets[0]"),
        ("checker_unsupported", "letter[1][0]"),
    ]


# AC1: the checker judges only what the code validator passed (a blocked draft costs no
# checker call).
def test_no_checker_call_when_validator_blocks(app_dsn: str, go: Go, profile_dir: Path) -> None:
    lead(app_dsn, go, profile_dir)
    bad = json.loads(json.dumps(GOOD))
    bad["roles"][0]["bullets"][0]["text"] = "Cut the invoice job from 40 to 5 minutes."
    provider = FakeProvider(answer(json.dumps(bad)), answer(json.dumps(LETTER)))
    run(app_dsn, profile_dir, provider)
    assert len(provider.requests) == 2


# AC2: a missing or extra verdict is malformed: one retry, then InvalidOutput (the draft
# fails; it is never ready_for_review).
def test_mismatched_verdicts_retry_then_fail(app_dsn: str, bank: Bank) -> None:
    c, pool = checker(FakeProvider(verdicts(1), verdicts(3)), app_dsn)
    with pool, pytest.raises(InvalidOutput):
        c.check(ITEMS, bank)


def test_mismatched_verdicts_once_is_retried(app_dsn: str, bank: Bank) -> None:
    provider = FakeProvider(
        verdicts(1), verdicts(2, {1: "unsupported"}), verdicts(2, {1: "unsupported"})
    )
    c, pool = checker(provider, app_dsn)
    with pool:
        found = c.check(ITEMS, bank)
    assert [(f.rule, f.where) for f in found] == [("checker_unsupported", "letter[0][0]")]
    assert len(provider.requests) == 3  # a retry, then the second run (M9-06)


# A closest id the bank doesn't hold is dropped (the page looks it up).
def test_unknown_closest_dropped(app_dsn: str, bank: Bank) -> None:
    v = {
        "verdicts": [
            {"n": 0, "verdict": "unsupported", "reason": "r", "closest": "made-up"},
            {"n": 1, "verdict": "unsupported", "reason": "r", "closest": "letter-availability"},
        ]
    }
    c, pool = checker(FakeProvider(answer(json.dumps(v)), answer(json.dumps(v))), app_dsn)
    with pool:
        found = c.check(ITEMS, bank)
    assert [f.closest for f in found] == [None, "letter-availability"]


def test_failed_draft_when_checker_malformed(app_dsn: str, go: Go, profile_dir: Path) -> None:
    posting = lead(app_dsn, go, profile_dir)
    provider = FakeProvider(
        answer(json.dumps(GOOD)), answer(json.dumps(LETTER)), verdicts(1), verdicts(1)
    )
    run(app_dsn, profile_dir, provider)
    d = latest(app_dsn, posting)
    assert d["status"] == "failed"
    assert d["error"].startswith("InvalidOutput")


# M9-06: the checker's verdicts vary run to run, so an unsupported sentence is blocked only
# when a second run flags it too. An altered fact (title, employer, date: what a background
# check compares) blocks on one run. The finding is the first run's; a clean first run needs
# no second.
@pytest.mark.parametrize(
    ("first", "second", "blocked", "calls"),
    [
        ({1: "unsupported"}, {1: "unsupported"}, ["letter[0][0]"], 2),
        ({1: "unsupported"}, {1: "altered_fact"}, ["letter[0][0]"], 2),  # both flag it
        ({1: "unsupported"}, {}, [], 2),  # the second run disagrees: not blocked
        ({0: "unsupported", 1: "unsupported"}, {1: "unsupported"}, ["letter[0][0]"], 2),
        ({1: "altered_fact"}, None, ["letter[0][0]"], 1),  # an altered fact: no second run
        ({0: "altered_fact", 1: "unsupported"}, {}, ["summary[0]"], 2),
        ({}, None, [], 1),  # nothing flagged: one call
    ],
)
def test_two_runs_must_agree(
    app_dsn: str,
    bank: Bank,
    first: dict[int, str],
    second: dict[int, str] | None,
    blocked: list[str],
    calls: int,
) -> None:
    answers = [verdicts(2, first)] + ([verdicts(2, second)] if second is not None else [])
    provider = FakeProvider(*answers)
    c, pool = checker(provider, app_dsn)
    with pool:
        found = c.check(ITEMS, bank)
    assert [f.where for f in found] == blocked
    assert [f.rule for f in found] == [  # the first run's verdict
        f"checker_{'altered_fact' if first[n] == 'altered_fact' else 'unsupported'}"
        for n in sorted(first)
        if ITEMS[n].where in blocked
    ]
    assert len(provider.requests) == calls
