"""The numbers a sentence states, normalised (M4-01 AC3, M4-04 AC5): a draft may only
use numbers its cited evidence lines contain.

"~53%" → 53 %, approximate; "5+ years" → 5 year, plus; "1.2k" → 1200; "40,000" → 40000.
Digits that are part of a name aren't numbers:
- glued to letters (OAuth2, S3, K8s, 3DS, Web3), after a hyphen (GPT-4) or a plus (N+1);
- a version after a known name ("Python 3", "Java 21").
"""

import re
from collections.abc import Iterable
from dataclasses import dataclass

from jobhunt_agents.aliases import normalize


@dataclass(frozen=True)
class Number:
    value: float
    unit: str  # "", "%", year, month, week, day, hour, minute, second, ms, x
    approx: bool = False  # "~", "about", "≈"
    plus: bool = False  # "5+"


# A number not glued to a letter, digit, dot, hyphen or plus on its left (N+1); its suffix.
_NUMBER = re.compile(r"(?<![\w.\-+])([~≈]?)(\d+(?:,\d{3})*(?:\.\d+)?)([A-Za-z%]*)(\+?)")
_SUFFIX_UNITS = {
    "%": "%",
    "h": "hour",
    "ms": "ms",
    "s": "second",
    "x": "x",
    "k": "k",
    "m": "minute",
}
_WORD_UNITS = {
    "percent": "%",
    "per cent": "%",
    "year": "year",
    "years": "year",
    "yr": "year",
    "yrs": "year",
    "month": "month",
    "months": "month",
    "week": "week",
    "weeks": "week",
    "day": "day",
    "days": "day",
    "hour": "hour",
    "hours": "hour",
    "minute": "minute",
    "minutes": "minute",
    "min": "minute",
    "mins": "minute",
    "second": "second",
    "seconds": "second",
    "ms": "ms",
    "times": "x",
}
_NEXT_WORD = re.compile(r"\s*([A-Za-z]+(?: cent)?)")


def numbers(text: str, names: Iterable[str] = ()) -> list[Number]:
    """The numbers text states, in order. `names` are skill or product names whose
    version numbers ("Python 3") are part of the name."""
    known = {normalize(n) for n in names if n}
    out: list[Number] = []
    for m in _NUMBER.finditer(text):
        approx, digits, suffix, plus = m.groups()
        if suffix and suffix.lower() not in _SUFFIX_UNITS:
            continue  # a name: 3DS, Web3 is excluded by the lookbehind
        before = text[: m.start()].rstrip()
        last_word = re.findall(r"[\w.+#-]+$", before)
        if last_word and normalize(last_word[0]) in known:
            continue  # a version: "Python 3"
        value = float(digits.replace(",", ""))
        unit = _SUFFIX_UNITS.get(suffix.lower(), "") if suffix else ""
        if unit == "k":
            value, unit = value * 1000, ""
        if not unit:
            nxt = _NEXT_WORD.match(text, m.end())
            if nxt and nxt.group(1).lower() in _WORD_UNITS:
                unit = _WORD_UNITS[nxt.group(1).lower()]
        approx_word = re.search(r"\b(about|around|roughly|approximately|nearly)\s*$", before, re.I)
        out.append(Number(value, unit, approx=bool(approx) or bool(approx_word), plus=bool(plus)))
    return out
