# ruamel.yaml is untyped (ADR-0019).
# pyright: reportUnknownMemberType=false, reportUnknownVariableType=false
"""The CV-variant check (M4-06): the owner's blind label per eval posting against the
variant `decide()` chose in the recorded reasons run. Measuring needs no model call."""

import json
from collections import Counter
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, cast

from ruamel.yaml import YAML

from jobhunt_agents.evals.data import EvalSet
from jobhunt_agents.variant import VARIANTS

UNANSWERED = "?"


def load_labels(path: Path) -> dict[str, str]:
    raw = cast(dict[str, Any], YAML(typ="safe").load(path.read_text()) or {})
    return {k: str(v.get("variant", UNANSWERED)).strip() for k, v in raw.items()}


def init_file(s: EvalSet, path: Path) -> None:
    """Writes one blank answer per real eval posting. Never shows the model's choice;
    refuses to overwrite a file that already has answers."""
    if path.exists() and any(v != UNANSWERED for v in load_labels(path).values()):
        raise FileExistsError(f"{path} already has answers; edit it instead")
    lines = [
        "# CV variant per eval posting (M4-06, OI-3). Label blind: this file never shows",
        "# the model's choice. variant: one of BE PAY DIST FS AI CONS",
        "#   BE backend · PAY payments/fintech · DIST distributed systems/infrastructure",
        "#   FS full-stack · AI AI/LLM/ML · CONS consulting or client-facing",
        '# note: optional, only for a close call ("PAY or BE").',
        "",
    ]
    for i in s.items():
        if i.fixture_of is not None:
            continue
        lines += [
            f"{i.id}:  # {i.company} — {i.title}",
            f'  variant: "{UNANSWERED}"',
            '  note: ""',
        ]
    path.write_text("\n".join(lines) + "\n")


@dataclass
class VariantReport:
    n: int
    agreement: float
    matrix: dict[str, dict[str, int]]  # owner's variant -> model's variant -> count
    model_share: dict[str, float] = field(default_factory=dict[str, float])

    def format(self) -> str:
        head = "owner \\ model " + "".join(f"{v:>6}" for v in VARIANTS)
        rows = [
            f"{o:<13} " + "".join(f"{self.matrix[o][m]:>6}" for m in VARIANTS) for o in VARIANTS
        ]
        share = " · ".join(f"{v} {self.model_share.get(v, 0):.0%}" for v in VARIANTS)
        return "\n".join(
            [head, *rows, f"agreement {self.agreement:.1%} · n={self.n}", f"model: {share}"]
        )


def report(labels: dict[str, str], model: dict[str, str]) -> VariantReport:
    """Owner labels against the model's choices, on the postings both have."""
    if missing := [k for k, v in labels.items() if v == UNANSWERED]:
        raise ValueError(f"{len(missing)} unanswered: {', '.join(missing[:5])}")
    if bad := [v for v in labels.values() if v not in VARIANTS]:
        raise ValueError(f"unknown variant {bad[0]!r}: want one of {' '.join(VARIANTS)}")
    both = [k for k in labels if k in model]
    matrix: dict[str, dict[str, int]] = {o: dict.fromkeys(VARIANTS, 0) for o in VARIANTS}
    for k in both:
        matrix[labels[k]][model[k]] += 1
    shares = Counter(model[k] for k in both)
    n = len(both)
    return VariantReport(
        n=n,
        agreement=sum(labels[k] == model[k] for k in both) / n if n else 0.0,
        matrix=matrix,
        model_share={v: c / n for v, c in shares.items()},
    )


def model_choices(run: Path) -> dict[str, str]:
    """cv_variant per item from a recorded reasons run (evals/private/reasons-vN.json)."""
    outputs = cast(dict[str, dict[str, Any]], json.loads(run.read_text())["outputs"])
    return {k: str(v["cv_variant"]) for k, v in outputs.items() if v.get("cv_variant")}
