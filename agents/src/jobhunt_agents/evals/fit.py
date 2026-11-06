"""Fit bands against the owner's blind fit labels (M3-06): a 5x5 confusion matrix, exact
and within-one-level agreement, and the band cut-offs that agree best on dev. Weights stay
as configured; only the cut-offs are fitted. No model call."""

from collections.abc import Sequence
from itertools import combinations, pairwise

from jobhunt_agents.score import FIT_LEVELS, Bands, FitLevel, fit_level

_SHORT = {
    "very_strong": "v.strong",
    "strong": "strong",
    "good": "good",
    "stretch": "stretch",
    "no": "no",
}


class Agreement:
    def __init__(
        self,
        points: Sequence[float],
        labels: Sequence[FitLevel],
        b: Bands,
        location: Sequence[float] | None = None,  # location points: the M3-09 gate applies
    ) -> None:
        idx = {lv: i for i, lv in enumerate(FIT_LEVELS)}
        self.n = len(labels)
        self.matrix = [[0] * len(FIT_LEVELS) for _ in FIT_LEVELS]
        d: list[int] = []
        locs = location if location is not None else [15.0] * len(points)  # 15: never capped
        for p, loc, want in zip(points, locs, labels, strict=True):
            got = fit_level(p, loc, b)
            self.matrix[idx[want]][idx[got]] += 1
            d.append(abs(idx[want] - idx[got]))
        self.exact = sum(x == 0 for x in d) / self.n if self.n else 0.0
        self.within_one = sum(x <= 1 for x in d) / self.n if self.n else 0.0
        self.distance = sum(d)

    def format(self) -> str:
        head = "owner \\ band " + "".join(f"{_SHORT[lv]:>9}" for lv in FIT_LEVELS)
        rows = [
            f"{_SHORT[lv]:<12} " + "".join(f"{c:>9}" for c in row)
            for lv, row in zip(FIT_LEVELS, self.matrix, strict=True)
        ]
        tail = f"exact {self.exact:.1%} · within one level {self.within_one:.1%} · n={self.n}"
        return "\n".join([head, *rows, tail])


def fit_bands(points: Sequence[float], labels: Sequence[FitLevel]) -> Bands:
    """The cut-offs with the best exact agreement, then within-one, then the smallest
    total level distance. Each candidate sits midway between two neighbouring scores
    (rounded to 0.5), as far as possible from the points it separates."""
    s = sorted(set(points))
    mids = [(a + b) / 2 for a, b in pairwise(s)]
    cands = sorted({round(c * 2) / 2 for c in [s[0] - 1, *mids, s[-1] + 1]}, reverse=True)
    best: tuple[tuple[float, float, int], Bands] | None = None
    for vs, st, gd, sr in combinations(cands, 4):  # descending, so always valid bands
        b = Bands(very_strong=vs, strong=st, good=gd, stretch=sr, location_gate=0)
        a = Agreement(points, labels, b)
        key = (a.exact, a.within_one, -a.distance)
        if best is None or key > best[0]:
            best = (key, b)
    if best is None:
        raise ValueError("need at least four distinct scores to fit four cut-offs")
    return best[1]
