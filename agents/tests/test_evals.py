"""M3-07: the eval set's tooling: text preparation, stratified sampling, labelling passes,
metrics and the CI replay mode. No network, no database."""

import json
from decimal import Decimal
from pathlib import Path
from typing import Any

import pytest

from jobhunt_agents.aliases import normalize
from jobhunt_agents.evals.data import (
    INJECTIONS,
    EvalSet,
    Lead,
    sample,
)
from jobhunt_agents.evals.label import label_extraction, label_fit, parse_field, prefill_all
from jobhunt_agents.evals.report import Thresholds, replay, run_live, score_run
from jobhunt_agents.extraction import Extraction
from jobhunt_agents.profile import Skill, SkillMap
from jobhunt_agents.score import FIT_LEVELS
from jobhunt_agents.text import clean_html, language, posting_text, terminal_safe

EXTRACTION = {
    "required_skills": ["Go", "PostgreSQL"],
    "optional_skills": ["Kubernetes"],
    "min_years": 3,
    "seniority": "mid",
    "visa_sponsorship": "unknown",
    "salary": None,
    "languages": ["English"],
    "remote_policy": "remote",
}


# --- text


def test_clean_html_keeps_structure_and_drops_scripts() -> None:
    html = (
        "&lt;p&gt;We build &amp;amp; ship.&lt;/p&gt;"
        "&lt;ul&gt;&lt;li&gt;Go&lt;/li&gt;&lt;li&gt;SQL&lt;/li&gt;&lt;/ul&gt;"
        "&lt;script&gt;alert(1)&lt;/script&gt;&lt;style&gt;p{}&lt;/style&gt;"
    )
    assert clean_html(html) == "We build & ship.\n- Go\n- SQL"


@pytest.mark.parametrize(
    ("source", "payload", "want"),
    [
        (
            "lever",
            {
                "text": "BE",
                "descriptionPlain": "About us",
                "lists": [{"text": "You have", "content": "<li>Go</li>"}],
                "additionalPlain": "Benefits",
            },
            ["About us", "You have", "- Go", "Benefits"],
        ),
        ("greenhouse", {"title": "BE", "content": "&lt;p&gt;Hello&lt;/p&gt;"}, ["Hello"]),
        ("ashby", {"title": "BE", "descriptionPlain": "Plain text"}, ["Plain text"]),
    ],
)
def test_posting_text_per_source(source: str, payload: dict[str, Any], want: list[str]) -> None:
    text = posting_text(source, payload, title="BE", location="Remote")
    assert text.startswith("Title: BE\nLocation: Remote\n")
    for w in want:
        assert w in text


def test_terminal_safe_strips_control_sequences() -> None:
    assert terminal_safe("ok\x1b[2J\x1b]0;title\x07 done\r\n") == "ok[2J]0;title done\n"


def test_language() -> None:
    assert language("Nous recherchons un développeur pour notre équipe et vous serez") == "fr"
    assert language("We are looking for an engineer to join our team and you will") == "en"


# --- sampling


def leads(n: int) -> list[Lead]:
    fams = ["backend", "frontend", "data", "ai_ml", "mobile", "platform"]
    return [
        Lead(
            posting_id=i,
            company=f"C{i % 9}",
            title=f"Engineer {i}",
            location="Remote",
            role_family=fams[i % len(fams)],
            source="greenhouse",
            payload={"title": "x", "content": "word " * (50 + i * 7)},
            language="fr" if i % 5 == 0 else "en",
        )
        for i in range(n)
    ]


# AC1: stratified: role families, EN and FR, short and long, several companies; fixed seed.
def test_sample_is_stratified_and_reproducible() -> None:
    pool = leads(300)
    picked = sample(pool, 36, seed=1)
    assert len(picked) == 36
    assert len({p.posting_id for p in picked}) == 36
    assert {p.role_family for p in picked} == {
        "backend",
        "frontend",
        "data",
        "ai_ml",
        "mobile",
        "platform",
    }
    assert {p.language for p in picked} == {"en", "fr"}
    assert max(sum(p.company == c for p in picked) for c in {p.company for p in picked}) <= 5
    lengths = sorted(len(p.text) for p in picked)
    assert lengths[0] * 3 < lengths[-1]  # short and long both present
    assert [p.posting_id for p in sample(pool, 36, seed=1)] == [p.posting_id for p in picked]


def test_create_set_adds_injection_fixtures(tmp_path: Path) -> None:
    s = EvalSet(tmp_path)
    s.create(sample(leads(120), 36, seed=1))
    items = s.items()
    assert len(items) == 36 + len(INJECTIONS)
    fixtures = [i for i in items if i.fixture_of is not None]
    assert len(fixtures) == len(INJECTIONS)
    for f in fixtures:
        assert f.injection is not None
        assert f.injection.text in s.text(f)
        assert f.injection.text not in s.text(s.item(f"p{f.fixture_of}"))
    # labels committed, posting texts local only
    assert (tmp_path / "labels").is_dir()
    assert (tmp_path / "postings").is_dir()
    assert not any("content" in p.read_text() for p in (tmp_path / "labels").iterdir())
    with pytest.raises(FileExistsError):
        s.create(sample(leads(120), 36, seed=1))


# --- labelling


def scripted(answers: list[str]) -> Any:
    it = iter(answers)
    shown: list[str] = []

    def ask(prompt: str) -> str:
        shown.append(prompt)
        return next(it)

    ask.shown = shown  # type: ignore[attr-defined]
    return ask


def small_set(tmp_path: Path, n: int = 3) -> EvalSet:
    s = EvalSet(tmp_path)
    s.create(sample(leads(60), n, seed=2), fixtures=False)
    return s


# AC1: the blind pass shows only the posting text; answers are saved as they are given,
# and a rerun resumes where it stopped.
def test_label_fit_blind_and_resumable(tmp_path: Path) -> None:
    s = small_set(tmp_path)
    out: list[str] = []
    label_fit(s, ask=scripted(["2", "q"]), show=out.append)
    first = s.items()[0]
    assert first.fit == "strong"
    assert s.items()[1].fit is None
    assert not any("prefill" in o or "score" in o.lower() for o in out)
    label_fit(s, ask=scripted(["5", "x", "1"]), show=out.append)  # "x" is re-asked
    assert [i.fit for i in s.items()] == ["strong", "no", "very_strong"]
    assert FIT_LEVELS[0] == "very_strong"


def test_label_fit_refuses_after_prefill(tmp_path: Path) -> None:
    s = small_set(tmp_path, 1)
    item = s.items()[0]
    s.save(item.model_copy(update={"prefill": Extraction.model_validate(EXTRACTION)}))
    with pytest.raises(SystemExit, match="blind"):
        label_fit(s, ask=scripted([]), show=lambda _: None)


# AC1: the extraction pass shows the pre-fill; the owner accepts or corrects each field;
# changed fields are recorded.
def test_label_extraction_records_changed_fields(tmp_path: Path) -> None:
    s = small_set(tmp_path, 1)
    item = s.items()[0]
    s.save(
        item.model_copy(update={"fit": "good", "prefill": Extraction.model_validate(EXTRACTION)})
    )
    # accept, correct optional_skills, accept, correct seniority (bad then good), accept x4
    label_extraction(
        s,
        ask=scripted(["", "Docker, Kubernetes", "", "senior-ish", "senior", "", "", "", ""]),
        show=lambda _: None,
    )
    got = s.items()[0]
    assert got.extraction is not None
    assert got.extraction.optional_skills == ["Docker", "Kubernetes"]
    assert got.extraction.seniority == "senior"
    assert got.extraction.required_skills == ["Go", "PostgreSQL"]
    assert got.changed_fields == ["optional_skills", "seniority"]
    assert got.labelled_by == "owner"


@pytest.mark.parametrize(
    ("field", "raw", "want"),
    [
        ("required_skills", "Go, SQL ,", ["Go", "SQL"]),
        ("required_skills", "-", []),
        ("min_years", "4", 4),
        ("min_years", "-", None),
        ("salary", "-", None),
        (
            "salary",
            "50000-70000 EUR year",
            {"min": 50000, "max": 70000, "currency": "EUR", "period": "year"},
        ),
        ("remote_policy", "Hybrid", "hybrid"),
    ],
)
def test_parse_field(field: str, raw: str, want: object) -> None:
    got = parse_field(field, raw)
    assert (got.model_dump() if hasattr(got, "model_dump") else got) == want  # type: ignore[union-attr]


@pytest.mark.parametrize(
    ("field", "raw"), [("min_years", "three"), ("seniority", "guru"), ("salary", "lots")]
)
def test_parse_field_rejects(field: str, raw: str) -> None:
    with pytest.raises(ValueError, match="want"):
        parse_field(field, raw)


# --- metrics and replay (AC2, AC3)


def labelled_set(tmp_path: Path) -> EvalSet:
    s = EvalSet(tmp_path)
    s.create(sample(leads(120), 4, seed=3))
    for item in s.items():
        if item.fixture_of is None:
            s.save(
                item.model_copy(
                    update={"fit": "good", "extraction": Extraction.model_validate(EXTRACTION)}
                )
            )
    return s


def run_file(
    s: EvalSet, path: Path, change: dict[str, Any] | None = None, injected: bool = False
) -> Path:
    outputs = {}
    for item in s.items():
        out: dict[str, Any] = dict(EXTRACTION)
        if change and item.fixture_of is None:
            out.update(change)
        if injected and item.injection is not None:  # the model followed the injection
            out["required_skills"] = [*out["required_skills"], item.injection.forbidden[0]]
        outputs[item.id] = {"extraction": out, "fit": "good"}
    path.write_text(
        json.dumps(
            {
                "prompt": "extract/v1.md",
                "model": "claude-haiku-5-5",
                "cost_usd": "0.0123",
                "outputs": outputs,
            }
        )
    )
    return path


def test_score_run_perfect(tmp_path: Path) -> None:
    s = labelled_set(tmp_path)
    r = score_run(s, json.loads(run_file(s, tmp_path / "run.json").read_text()))
    assert r.field_accuracy["required_skills"] == 1.0
    assert r.field_accuracy["seniority"] == 1.0
    assert r.fit_agreement == 1.0
    assert r.injection_passed == r.injection_total == len(INJECTIONS)
    assert str(r.cost_usd) == "0.0123"


def test_score_run_catches_errors_and_injection(tmp_path: Path) -> None:
    s = labelled_set(tmp_path)
    run = run_file(
        s,
        tmp_path / "run.json",
        change={"seniority": "senior", "required_skills": ["Elixir"]},
        injected=True,
    )
    r = score_run(s, json.loads(run.read_text()))
    assert r.field_accuracy["seniority"] == 0.0
    assert r.field_accuracy["required_skills"] == 0.0  # {Elixir} vs {Go, PostgreSQL}
    assert r.items == len([i for i in s.items() if i.extraction is not None])
    assert r.injection_passed == 0


# AC3: replay re-scores recorded runs without network; it fails below the thresholds,
# and passes (reporting only) while no thresholds are set.
def test_replay_thresholds(tmp_path: Path) -> None:
    s = labelled_set(tmp_path)
    runs = tmp_path / "runs"
    runs.mkdir()
    run_file(s, runs / "baseline.json", change={"seniority": "senior"})
    assert replay(s, runs, thresholds=None, show=lambda _: None) == 0
    strict = Thresholds(
        field_accuracy={"seniority": 0.9}, fit_agreement=0.0, injection_pass_rate=1.0
    )
    assert replay(s, runs, thresholds=strict, show=lambda _: None) == 1


def test_replay_with_nothing_recorded(tmp_path: Path) -> None:
    out: list[str] = []
    assert replay(labelled_set(tmp_path), tmp_path / "none", thresholds=None, show=out.append) == 0
    assert "no recorded runs" in out[0]


# --- pre-fill (AC1): only after the blind pass; the total cost is shown first and must be
# under the limit; nothing is called when it isn't.


def test_prefill_needs_the_blind_pass_first(tmp_path: Path) -> None:
    s = small_set(tmp_path, 2)
    with pytest.raises(SystemExit, match="blind pass"):
        prefill_all(
            s,
            extract=fail_extract,
            estimate=lambda texts: Decimal("0.01"),
            ask=scripted([]),
            show=lambda _: None,
        )


def fail_extract(text: str, posting_id: int) -> Extraction:
    raise AssertionError("must not be called")


def fit_all(s: EvalSet) -> None:
    for item in s.items():
        s.save(item.model_copy(update={"fit": "good"}))


def test_prefill_refuses_over_the_limit(tmp_path: Path) -> None:
    s = small_set(tmp_path, 2)
    fit_all(s)
    out: list[str] = []
    with pytest.raises(SystemExit, match="limit"):
        prefill_all(
            s,
            extract=fail_extract,
            estimate=lambda texts: Decimal("0.11"),
            ask=scripted([]),
            show=out.append,
        )
    assert "$0.11" in out[0]


def test_prefill_after_confirmation(tmp_path: Path) -> None:
    s = small_set(tmp_path, 2)
    fit_all(s)
    calls: list[int] = []

    def extract(text: str, posting_id: int) -> Extraction:
        calls.append(posting_id)
        return Extraction.model_validate(EXTRACTION)

    out: list[str] = []
    prefill_all(
        s,
        extract=extract,
        estimate=lambda texts: Decimal("0.02"),
        ask=scripted(["y"]),
        show=out.append,
    )
    assert "$0.02" in out[0]
    assert len(calls) == 2
    assert all(i.prefill == Extraction.model_validate(EXTRACTION) for i in s.items())
    prefill_all(
        s,
        extract=fail_extract,
        estimate=lambda texts: Decimal(0),
        ask=scripted([]),
        show=out.append,
    )  # nothing left


def test_prefill_declined(tmp_path: Path) -> None:
    s = small_set(tmp_path, 1)
    fit_all(s)
    prefill_all(
        s,
        extract=fail_extract,
        estimate=lambda texts: Decimal("0.01"),
        ask=scripted(["n"]),
        show=lambda _: None,
    )
    assert s.items()[0].prefill is None


# --- a live run (AC2) records its outputs for replay


def test_run_live_records_outputs(tmp_path: Path) -> None:
    s = labelled_set(tmp_path)
    runs = tmp_path / "runs"
    path = run_live(
        s,
        extract=lambda text, posting_id: Extraction.model_validate(EXTRACTION),
        spent=iter([Decimal("1.000"), Decimal("1.025")]).__next__,
        runs=runs,
        name="baseline",
        prompt="extract/v1.md",
        model="claude-haiku-5-5",
    )
    run = json.loads(path.read_text())
    assert set(run["outputs"]) == {i.id for i in s.items()}
    assert run["cost_usd"] == "0.025"
    assert score_run(s, run).field_accuracy["required_skills"] == 1.0


# AC1: every extraction answer is saved as it is given; quitting mid-posting loses nothing.
def test_label_extraction_saves_each_field_and_resumes(tmp_path: Path) -> None:
    s = small_set(tmp_path, 1)
    item = s.items()[0]
    s.save(
        item.model_copy(update={"fit": "good", "prefill": Extraction.model_validate(EXTRACTION)})
    )
    ask = scripted(["Go, Rust", ""])  # two fields answered, then the owner quits
    with pytest.raises(StopIteration):
        label_extraction(s, ask=ask, show=lambda _: None)
    assert s.items()[0].extraction is None
    assert s.items()[0].draft == {
        "required_skills": ["Go", "Rust"],
        "optional_skills": ["Kubernetes"],
    }

    ask = scripted(["", "", "", "", "", ""])  # resumes at min_years
    label_extraction(s, ask=ask, show=lambda _: None)
    assert ask.shown[0].startswith("min_years")
    got = s.items()[0]
    assert got.extraction is not None
    assert got.extraction.required_skills == ["Go", "Rust"]
    assert got.changed_fields == ["required_skills"]
    assert got.draft == {}


# AC2: skill lists score by precision, recall and F1 after folding and skill-map aliases,
# so wording ("Golang", "postgres") isn't counted as a mistake.
def test_score_run_skill_lists_use_aliases(tmp_path: Path) -> None:
    s = labelled_set(tmp_path)
    skills = SkillMap(
        skills=(),
        by_key={
            normalize(k): Skill(name=n, level="proven", evidence="x")
            for n, keys in {"Go": [], "PostgreSQL": ["pg"]}.items()
            for k in [n, *keys]
        },
    )
    run = json.loads(
        run_file(
            s, tmp_path / "run.json", change={"required_skills": ["golang", "pg", "Rust"]}
        ).read_text()
    )
    plain = score_run(s, run)
    assert plain.field_precision["required_skills"] == pytest.approx(1 / 3)  # golang: the table
    r = score_run(s, run, skills=skills)
    assert r.field_precision["required_skills"] == pytest.approx(2 / 3)
    assert r.field_recall["required_skills"] == 1.0
    assert r.field_accuracy["required_skills"] == pytest.approx(0.8)  # F1


# M3-04: prompt work is judged on dev only; held-out runs once at the end. The split is
# fixed by a seed, spread over role family and fit, and keeps fixture bases in dev.
def test_split_dev_and_heldout(tmp_path: Path) -> None:
    s = EvalSet(tmp_path)
    s.create(sample(leads(200), 20, seed=4))
    for k, item in enumerate(i for i in s.items() if i.fixture_of is None):
        s.save(item.model_copy(update={"fit": FIT_LEVELS[k % len(FIT_LEVELS)]}))
    split = s.make_split(heldout=6, seed=7)
    assert s.make_split(heldout=6, seed=7) == split  # reproducible
    assert json.loads((tmp_path / "split.json").read_text()) == split
    real = {i.id for i in s.items() if i.fixture_of is None}
    assert len(split["heldout"]) == 6
    assert set(split["dev"]) | set(split["heldout"]) == real
    assert not set(split["dev"]) & set(split["heldout"])
    bases = {f"p{i.fixture_of}" for i in s.items() if i.fixture_of is not None}
    assert not bases & set(split["heldout"])
    by_id = {i.id: i for i in s.items()}
    held = [by_id[i] for i in split["heldout"]]
    assert len({i.role_family for i in held}) >= 3
    assert len({i.fit for i in held}) >= 3

    dev = {i.id for i in s.subset("dev")}
    assert dev == set(split["dev"]) | {i.id for i in s.items() if i.fixture_of is not None}
    held_run = {i.id for i in s.subset("heldout")}
    assert set(split["heldout"]) | bases <= held_run  # fixtures need their base's output
    assert s.scored("heldout") == set(split["heldout"])
    assert s.scored("all") == real
    assert {i.id for i in s.subset("all")} == {i.id for i in s.items()}


def test_score_run_scores_only_the_runs_subset(tmp_path: Path) -> None:
    s = labelled_set(tmp_path)
    run = json.loads(run_file(s, tmp_path / "run.json", change={"seniority": "senior"}).read_text())
    keep = next(i.id for i in s.items() if i.fixture_of is None)
    run["scored"] = [keep]
    r = score_run(s, run)
    assert r.items == 1
    assert r.injection_total == len(INJECTIONS)  # fixtures always count


def test_run_live_records_the_subset(tmp_path: Path) -> None:
    s = labelled_set(tmp_path)
    split = s.make_split(heldout=1, seed=1)
    path = run_live(
        s,
        subset="heldout",
        extract=lambda text, posting_id: Extraction.model_validate(EXTRACTION),
        spent=iter([Decimal(0), Decimal(0)]).__next__,
        runs=tmp_path / "runs",
        name="heldout-v1",
        prompt="extract/v1.md",
        model="claude-haiku-5-5",
    )
    run = json.loads(path.read_text())
    assert set(run["outputs"]) == {i.id for i in s.subset("heldout")}
    assert run["scored"] == split["heldout"]


# M3-04 AC3 / owner's hard gate: an injection followed in any way fails the run, with or
# without thresholds; injected years or visa count, not only injected skill names.
def test_injection_is_a_hard_gate(tmp_path: Path) -> None:
    s = labelled_set(tmp_path)
    runs = tmp_path / "runs"
    runs.mkdir()
    run_file(s, runs / "bad.json", injected=True)
    out: list[str] = []
    assert replay(s, runs, thresholds=None, show=out.append) == 1
    assert any("INJECTION" in line for line in out)


def test_injected_years_or_visa_fail_the_check(tmp_path: Path) -> None:
    s = labelled_set(tmp_path)
    run = json.loads(run_file(s, tmp_path / "run.json").read_text())
    for item in s.items():
        if item.injection is not None:
            run["outputs"][item.id]["extraction"] |= {"min_years": 7, "visa_sponsorship": "yes"}
    assert score_run(s, run).injection_passed == 0


def test_thresholds_on_precision(tmp_path: Path) -> None:
    s = labelled_set(tmp_path)
    run = json.loads(
        run_file(
            s, tmp_path / "run.json", change={"required_skills": ["Go", "PostgreSQL", "Elixir"]}
        ).read_text()
    )
    t = Thresholds(
        field_accuracy={"required_skills": 0.75},
        field_precision={"required_skills": 0.85},
        fit_agreement=0.0,
        injection_pass_rate=1.0,
    )
    fails = score_run(s, run).failures(t)
    assert fails == ["required_skills precision: 0.67 < 0.85"]


# Thresholds gate the run they name (the shipped prompt's dev run); other recorded runs
# are reported only, but the injection gate applies to every run.
def test_thresholds_gate_only_the_named_run(tmp_path: Path) -> None:
    s = labelled_set(tmp_path)
    runs = tmp_path / "runs"
    runs.mkdir()
    run_file(s, runs / "dev-extract-v1.json", change={"seniority": "senior"})
    run_file(s, runs / "dev-extract-v2.json")
    t = Thresholds(
        gate="dev-extract-v2",
        field_accuracy={"seniority": 0.9},
        fit_agreement=0.0,
        injection_pass_rate=1.0,
    )
    assert replay(s, runs, thresholds=t, show=lambda _: None) == 0
    t = t.model_copy(update={"gate": "dev-extract-v1"})
    assert replay(s, runs, thresholds=t, show=lambda _: None) == 1


def test_score_run_uses_the_tech_alias_table(tmp_path: Path) -> None:
    s = labelled_set(tmp_path)
    run = json.loads(
        run_file(
            s, tmp_path / "run.json", change={"required_skills": ["golang", "Postgres"]}
        ).read_text()
    )
    assert score_run(s, run).field_accuracy["required_skills"] == 1.0  # no skill map needed
