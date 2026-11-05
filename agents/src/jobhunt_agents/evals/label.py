"""The two labelling passes (M3-07 AC1), one posting at a time in the terminal.

1. Blind: the owner sets the fit level, seeing only the prepared posting text. Refused
   once any model output exists, so it stays blind.
2. Extraction: a model pre-fills the fields (cost shown and capped first); the owner
   accepts or corrects each one, and the fields he changed are recorded.

Answers are saved after every posting, so a pass can stop and resume.
"""

import re
from collections.abc import Callable
from decimal import Decimal

from jobhunt_agents.evals.data import FIT_LEVELS, EvalSet, Item
from jobhunt_agents.extraction import FIELDS, LIST_FIELDS, Extraction, Salary
from jobhunt_agents.profile import normalize
from jobhunt_agents.text import terminal_safe

Ask = Callable[[str], str]
Show = Callable[[str], None]

PREFILL_LIMIT_USD = Decimal("0.10")
_FIT_KEYS = {str(i + 1): level for i, level in enumerate(FIT_LEVELS)}
_FIT_PROMPT = "fit? 1 very strong · 2 strong · 3 good · 4 stretch · 5 no · s skip · q quit: "


def _header(n: int, total: int, item: Item) -> str:
    line = "=" * 78
    where = f"[{n}/{total}] {item.id} · {item.company} · {item.role_family} · {item.language}"
    return terminal_safe(f"\n{line}\n{where}\n{line}")


def label_fit(s: EvalSet, *, ask: Ask, show: Show) -> None:
    items = [i for i in s.items() if i.fixture_of is None]
    if any(i.prefill is not None for i in items):
        raise SystemExit("fit labels must be blind: model pre-fills already exist for this set")
    todo = [i for i in items if i.fit is None]
    for n, item in enumerate(todo, 1):
        show(_header(n, len(todo), item))
        show(terminal_safe(s.text(item)))
        while True:
            answer = ask(_FIT_PROMPT).strip().casefold()
            if answer == "q":
                return
            if answer == "s":
                break
            if answer in _FIT_KEYS:
                s.save(item.model_copy(update={"fit": _FIT_KEYS[answer]}))
                break


def prefill_all(
    s: EvalSet,
    *,
    extract: Callable[[str, int], Extraction],
    estimate: Callable[[list[str]], Decimal],
    ask: Ask,
    show: Show,
    model: str = "",
    prompt: str = "",
) -> None:
    items = [i for i in s.items() if i.fixture_of is None]
    if any(i.fit is None for i in items):
        raise SystemExit("finish the blind pass first (make evals-label PASS=fit)")
    todo = [i for i in items if i.prefill is None]
    if not todo:
        return
    texts = [s.text(i) for i in todo]
    cost = estimate(texts)
    show(
        f"pre-filling {len(todo)} postings: estimated cost ${cost:.4f} (limit ${PREFILL_LIMIT_USD})"
    )
    if cost >= PREFILL_LIMIT_USD:
        raise SystemExit(
            f"estimated cost is over the ${PREFILL_LIMIT_USD} limit; nothing was called"
        )
    if ask("go ahead? [y/N] ").strip().casefold() != "y":
        return
    for item, text in zip(todo, texts, strict=True):
        try:
            got = extract(text, item.posting_id)
        except Exception as e:  # noqa: BLE001 - label it by hand instead
            show(f"{item.id}: pre-fill failed ({type(e).__name__}); label it by hand")
            got = _blank()
        s.save(
            item.model_copy(
                update={"prefill": got, "prefill_model": model, "prefill_prompt": prompt}
            )
        )


def _blank() -> Extraction:
    return Extraction(
        required_skills=[],
        optional_skills=[],
        min_years=None,
        seniority="unknown",
        visa_sponsorship="unknown",
        salary=None,
        languages=[],
        remote_policy="unknown",
    )


_SALARY = re.compile(r"^(\d+)\s*-\s*(\d+)\s+([A-Za-z]{3})\s+(year|month|day|hour)$")
_ENUMS = {
    name: Extraction.model_fields[name].annotation
    for name in ("seniority", "visa_sponsorship", "remote_policy")
}


def _choices(field: str) -> tuple[str, ...]:
    return tuple(getattr(_ENUMS[field], "__args__", ()))


def parse_field(field: str, raw: str) -> object:
    """One typed answer; "-" is empty (lists) or none (years, salary)."""
    raw = raw.strip()
    if field in LIST_FIELDS:
        return [] if raw == "-" else [p.strip() for p in raw.split(",") if p.strip()]
    if field == "min_years":
        if raw == "-":
            return None
        if not raw.isdigit() or int(raw) > 50:
            raise ValueError("want a whole number of years (0-50) or -")
        return int(raw)
    if field == "salary":
        if raw == "-":
            return None
        m = _SALARY.match(raw)
        if not m:
            raise ValueError("want MIN-MAX CUR PERIOD, e.g. 50000-70000 EUR year, or -")
        return Salary(min=int(m[1]), max=int(m[2]), currency=m[3].upper(), period=m[4])  # type: ignore[arg-type]
    value = raw.casefold()
    if value not in _choices(field):
        raise ValueError(f"want one of {', '.join(_choices(field))}")
    return value


def _show_value(v: object) -> str:
    if isinstance(v, list):
        return ", ".join(map(str, v)) or "-"  # pyright: ignore[reportUnknownArgumentType]
    if isinstance(v, Salary):
        return f"{v.min}-{v.max} {v.currency} {v.period}"
    return "-" if v is None else str(v)


def changed_fields(final: Extraction, prefill: Extraction) -> list[str]:
    """Fields the label changed; lists compare as sets of folded names, so reordering or
    re-casing a skill is not a correction."""

    def key(f: str, e: Extraction) -> object:
        v = getattr(e, f)
        return {normalize(x) for x in v} if f in LIST_FIELDS else v

    return [f for f in FIELDS if key(f, final) != key(f, prefill)]


def label_extraction(s: EvalSet, *, ask: Ask, show: Show) -> None:
    todo = [
        i
        for i in s.items()
        if i.fixture_of is None and i.prefill is not None and i.extraction is None
    ]
    for n, start in enumerate(todo, 1):
        item, prefill = start, start.prefill
        if prefill is None:
            continue
        show(_header(n, len(todo), item))
        show(terminal_safe(s.text(item)))
        show("\nEnter accepts the pre-filled value; type a new one to correct it; - for empty.")
        values: dict[str, object] = dict(item.draft)  # answers saved before a quit
        for field in FIELDS:
            if field in values:
                continue
            pre = getattr(prefill, field)
            hint = f" ({' | '.join(_choices(field))})" if field in _ENUMS else ""
            while True:
                answer = ask(terminal_safe(f"{field}{hint} [{_show_value(pre)}]: "))
                if not answer.strip():
                    values[field] = pre
                    break
                try:
                    values[field] = parse_field(field, answer)
                    break
                except ValueError as e:
                    show(f"  {e}")
            draft = {k: v.model_dump() if isinstance(v, Salary) else v for k, v in values.items()}
            item = item.model_copy(update={"draft": draft})
            s.save(item)  # every answer is kept, even if the pass stops mid-posting
        final = Extraction.model_validate(item.draft)
        changed = changed_fields(final, prefill)
        s.save(
            item.model_copy(
                update={
                    "extraction": final,
                    "changed_fields": changed,
                    "labelled_by": "owner",
                    "draft": {},
                }
            )
        )
