"""M4-06: the owner's blind CV-variant labels against the model's choice (no model call)."""

from pathlib import Path

import pytest

from jobhunt_agents.evals.data import EvalSet, sample
from jobhunt_agents.evals.variants import VariantReport, init_file, load_labels, report
from tests.test_evals import leads


@pytest.fixture
def evalset(tmp_path: Path) -> EvalSet:
    s = EvalSet(tmp_path)
    s.create(sample(leads(120), 6, seed=1))
    return s


# AC1: one entry per real posting (fixtures left out), company and title as a comment,
# an empty answer, and never the model's choice.
def test_init_writes_blank_answers(evalset: EvalSet) -> None:
    path = evalset.dir / "variants.yaml"
    init_file(evalset, path)
    text = path.read_text()
    real = [i for i in evalset.items() if i.fixture_of is None]
    for i in real:
        assert f"{i.id}:" in text
        assert f"# {i.company} — {i.title}" in text
    assert text.count('variant: "?"') == len(real)
    assert "cv_variant" not in text
    assert set(load_labels(path)) == {i.id for i in real}


# AC1: answers already given are never overwritten.
def test_init_refuses_to_overwrite_answers(evalset: EvalSet) -> None:
    path = evalset.dir / "variants.yaml"
    init_file(evalset, path)
    first = next(i.id for i in evalset.items() if i.fixture_of is None)
    path.write_text(path.read_text().replace('variant: "?"', 'variant: "BE"', 1))
    with pytest.raises(FileExistsError, match="answers"):
        init_file(evalset, path)
    assert load_labels(path)[first] == "BE"


# AC2: the owner's label against the model's, a 6x6 matrix, agreement and shares;
# refused while answers are missing; an unknown variant is an error.
def test_report_matrix_and_agreement() -> None:
    labels = {"p1": "BE", "p2": "BE", "p3": "PAY", "p4": "AI"}
    model = {"p1": "BE", "p2": "DIST", "p3": "PAY", "p4": "DIST", "p9": "FS"}
    r = report(labels, model)
    assert isinstance(r, VariantReport)
    assert r.n == 4
    assert r.agreement == pytest.approx(0.5)
    assert r.matrix["BE"]["DIST"] == 1
    assert r.matrix["AI"]["DIST"] == 1
    assert r.model_share["DIST"] == pytest.approx(0.5)
    with pytest.raises(ValueError, match="unanswered"):
        report(labels | {"p5": "?"}, model)
    with pytest.raises(ValueError, match="unknown variant"):
        report({"p1": "BACKEND"}, model)
