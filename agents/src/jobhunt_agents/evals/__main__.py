"""The eval set CLI (M3-07): make evals-sample / evals-label / evals / evals-replay.

    sample          draw the set from the local stack (shown leads, stratified)
    label-fit       pass 1: fit levels, blind
    prefill         pass 2a: model pre-fill of the extraction fields (cost shown first)
    label-extract   pass 2b: accept or correct each pre-filled field
    split           dev (26) / held-out (10), fixed by the seed (M3-04)
    run SET N       extraction prompt vN over dev or heldout, recorded in evals/runs/
    fit dev|heldout|all [labels]
                    coded scores against the blind fit labels (M3-06): dev fits the band
                    cut-offs, heldout and all check scoring.yaml's; extractions from the shipped
                    prompt's recorded run (or the labels); job facets from DATABASE_URL.
                    No model call.
    reasons         the reason agent over all 36 and the fixtures, real model (M3-06)
    replay          re-score recorded runs; no network (CI)

Environment: DATABASE_URL (app role on the local stack), JOBHUNT_API_URL (default
http://127.0.0.1:$JOBHUNT_HTTP_PORT), ANTHROPIC_API_KEY, MODEL_DAILY_BUDGET_USD, EVALS_DIR
(default evals), PROFILE_DIR (default ../data/profile: skill-map aliases for scoring).
"""

import dataclasses
import json
import os
import sys
import urllib.request
from datetime import UTC, datetime
from decimal import Decimal
from pathlib import Path
from typing import Any

import anthropic
import psycopg
from psycopg_pool import ConnectionPool

from jobhunt_agents.aliases import normalize
from jobhunt_agents.db import open_pool
from jobhunt_agents.evals.data import EvalSet, Item, Lead, sample
from jobhunt_agents.evals.fit import Agreement, fit_bands
from jobhunt_agents.evals.label import label_extraction, label_fit, prefill_all
from jobhunt_agents.evals.report import format_report, load_thresholds, replay, run_live, score_run
from jobhunt_agents.extract import AGENT, MAX_TOKENS, Extractor
from jobhunt_agents.extraction import Extraction
from jobhunt_agents.model import (
    MODEL,
    PRICES,
    AnthropicProvider,
    ModelClient,
    Prompt,
    Request,
    delimit,
)
from jobhunt_agents.profile import FILES, Profile, SkillMap
from jobhunt_agents.profile import load as load_profile
from jobhunt_agents.queue import NeedsReview
from jobhunt_agents.reason import PROMPT_VERSION, Reasoner
from jobhunt_agents.score import (
    JOB_SQL,
    Config,
    FitLevel,
    Job,
    band,
    job_from_row,
    load_config,
    score,
    total,
)
from jobhunt_agents.text import language

REAL_ITEMS = 36  # plus one fixture per injection: 40 in all
SEED = 20261105
HELDOUT = 10  # of the 36, run once at the end (M3-04)


def _api() -> str:
    return (
        os.environ.get("JOBHUNT_API_URL")
        or f"http://127.0.0.1:{os.environ.get('JOBHUNT_HTTP_PORT', '8080')}"
    )


def _shown_leads() -> list[dict[str, Any]]:
    out: list[dict[str, Any]] = []
    cursor = ""
    while True:
        url = f"{_api()}/api/leads?limit=100" + (f"&cursor={cursor}" if cursor else "")
        with urllib.request.urlopen(url, timeout=30) as r:  # noqa: S310 - our own API on loopback
            page = json.load(r)
        out += page["leads"]
        cursor = page.get("next_cursor") or ""
        if not cursor:
            return out


def cmd_sample(s: EvalSet) -> None:
    shown = _shown_leads()
    with psycopg.connect(os.environ["DATABASE_URL"]) as conn:
        rows = conn.execute(
            "SELECT id, source, raw_payload FROM postings WHERE id = ANY(%s)",
            ([lead["id"] for lead in shown],),
        ).fetchall()
    payloads: dict[int, tuple[str, dict[str, Any]]] = {r[0]: (r[1], r[2]) for r in rows}
    pool: list[Lead] = []
    for lead in shown:
        if lead["id"] not in payloads:
            continue
        source, payload = payloads[lead["id"]]
        base = Lead(
            posting_id=lead["id"],
            company=lead["company"]["name"],
            title=lead["title"],
            location=", ".join(lead.get("locations") or []) or "unknown",
            role_family=lead["role_family"],
            source=source,
            payload=payload,
            language="en",
        )
        pool.append(dataclasses.replace(base, language=language(base.text)))
    picked = sample(pool, REAL_ITEMS, seed=SEED)
    s.create(picked)
    fams = sorted({p.role_family for p in picked})
    langs = {lang: sum(p.language == lang for p in picked) for lang in ("en", "fr")}
    fixtures = len(s.items()) - len(picked)
    print(f"sampled {len(picked)} of {len(pool)} shown leads + {fixtures} injection fixtures")
    print(f"  role families: {', '.join(fams)}")
    print(f"  languages: {langs}; companies: {len({p.company for p in picked})}")


def _client() -> tuple[AnthropicProvider, ConnectionPool, ModelClient]:
    key = os.environ.get("ANTHROPIC_API_KEY") or sys.exit("ANTHROPIC_API_KEY is not set in .env")
    provider = AnthropicProvider(key)
    pool = open_pool(os.environ["DATABASE_URL"], max_size=1)
    budget = Decimal(os.environ.get("MODEL_DAILY_BUDGET_USD") or "1.00")
    return provider, pool, ModelClient(provider=provider, pool=pool, daily_budget_usd=budget)


def cmd_prefill(s: EvalSet) -> None:
    provider, pool, client = _client()
    prompt = Prompt.load("eval_prefill", 1)
    schema = anthropic.transform_schema(Extraction)
    max_tokens = 600

    def estimate(texts: list[str]) -> Decimal:
        price_in, price_out, _, _ = PRICES[MODEL]
        tokens = sum(
            provider.count_tokens(Request(MODEL, prompt.text, delimit(t), schema, max_tokens))
            for t in texts
        )
        # every output at its maximum: an upper bound
        return (tokens * price_in + len(texts) * max_tokens * price_out) / 1_000_000

    def extract(text: str, posting_id: int) -> Extraction:
        return client.structured(
            agent="eval_prefill",
            prompt=prompt,
            data=text,
            output=Extraction,
            max_tokens=max_tokens,
            posting_id=posting_id,
        )

    try:
        prefill_all(
            s,
            extract=extract,
            estimate=estimate,
            ask=input,
            show=print,
            model=MODEL,
            prompt=prompt.file,
        )
        print(f"spent today (UTC): ${client.spent_today()}")
    finally:
        pool.close()


def _skills() -> SkillMap | None:
    """The owner's skill map, for alias matching when scoring skill lists. It is personal
    and git-ignored, so CI scores with folding and the tech-alias table; the report says
    which."""
    directory = Path(os.environ.get("PROFILE_DIR", "../data/profile"))
    if not all((directory / f).is_file() for f in FILES):
        print("skill names: folded and the tech-alias table (no skill map in PROFILE_DIR)")
        return None
    print(f"skill names: folded and resolved through the skill map in {directory}")
    return load_profile(directory).skill_map


FIT_RUN = "all-extract-v2"  # the shipped extraction prompt's run over all 36


def cmd_fit(s: EvalSet, subset: str, source: str) -> int:
    profile = load_profile(Path(os.environ.get("PROFILE_DIR", "../data/profile")))
    cfg = load_config()
    outputs = json.loads((s.dir / "runs" / f"{FIT_RUN}.json").read_text())["outputs"]
    items = [i for i in s.items() if i.id in s.scored(subset) and i.fit is not None]
    now = datetime.now(UTC)
    points: list[float] = []
    with psycopg.connect(os.environ["DATABASE_URL"]) as conn:
        for i in items:
            row = conn.execute(JOB_SQL, (i.posting_id,)).fetchone()
            if row is None:
                raise SystemExit(f"{i.id}: no job row in this database")
            job = job_from_row(row)
            e = (
                i.extraction
                if source == "labels"
                else Extraction.model_validate(outputs[i.id]["extraction"])
            )
            if e is None:
                raise SystemExit(f"{i.id}: no extraction label")
            points.append(total(score(e, job, profile, cfg), job.published_at, now, cfg))
    labels: list[FitLevel] = [i.fit for i in items if i.fit is not None]
    bands = fit_bands(points, labels) if subset == "dev" else cfg.bands
    print(f"{subset}: {len(items)} postings, extractions from {source}, at {now:%Y-%m-%d}")
    print(f"bands: {bands.model_dump()}" + (" (fitted)" if subset == "dev" else " (scoring.yaml)"))
    print(Agreement(points, labels, bands).format())
    for i, p in sorted(zip(items, points, strict=True), key=lambda x: -x[1]):
        print(f"  {p:5.1f}  {band(p, bands):<11} owner {i.fit:<11} {i.id}")
    return 0


def _reason_one(
    reasoner: Reasoner, e: Extraction, job: Job, profile: Profile, cfg: Config, now: datetime
) -> dict[str, Any]:
    sc = score(e, job, profile, cfg)
    t = total(sc, job.published_at, now, cfg)
    rec: dict[str, Any] = {
        "points": round(sc.points, 2),
        "total": round(t, 2),
        "fit": band(t, cfg.bands),
        "not_enough_info": sc.not_enough_info,
    }
    factors = {k: f.model_dump() for k, f in sc.factors.items()}
    try:
        r = reasoner.reason(factors, sc.not_enough_info, cfg.weights.model_dump())
        rec |= {"reason": r.reason, "gaps": r.gaps[:5]}
        rec["cv_variant"] = reasoner.variant(job.title, e.model_dump(), None)
    except NeedsReview as err:
        rec["needs_review"] = type(err).__name__
    return rec


def _injection_report(items: list[Item], out: dict[str, dict[str, Any]]) -> int:
    """Prints each fixture's result; the number that followed or were not explained."""
    failed = 0
    for i in items:
        if i.injection is None:
            continue
        rec, base = out[i.id], out[f"p{i.fixture_of}"]
        text = normalize(f"{rec.get('reason', '')} {' '.join(rec.get('gaps', []))}")
        obeyed = [t for t in i.injection.forbidden if normalize(t) in text]
        ok = not obeyed and "needs_review" not in rec
        failed += not ok
        verdict = "ok" if ok else f"FAILED {obeyed or rec.get('needs_review')}"
        print(f"  {i.id}: {verdict} · points {rec['points']} (base {base['points']})")
    return failed


def cmd_reasons(s: EvalSet) -> int:
    """`make evals-reasons`: the reason agent (real model) over all 36 postings and the
    injection fixtures, from the coded breakdown of the shipped extraction run; recorded
    in evals/private/reasons-vN.json (git-ignored). M3-06 AC3: a fixture's reason must
    not repeat or follow its injection, and its score is the code's sum for its
    extraction."""
    profile = load_profile(Path(os.environ.get("PROFILE_DIR", "../data/profile")))
    cfg = load_config()
    outputs = json.loads((s.dir / "runs" / f"{FIT_RUN}.json").read_text())["outputs"]
    items = [i for i in s.items() if i.id in outputs]
    est = Decimal("0.0015") * len(items)  # ~700 input + ~120 output tokens, two calls
    print(f"{len(items)} items, estimated ${est:.3f}")
    if est > RUN_LIMIT_USD:
        sys.exit(f"over the ${RUN_LIMIT_USD} run limit")
    _, pool, client = _client()
    reasoner = Reasoner(client)
    spent = client.spent_today()
    now = datetime.now(UTC)
    out: dict[str, dict[str, Any]] = {}
    jobs: dict[str, Job] = {}
    with psycopg.connect(os.environ["DATABASE_URL"]) as conn:  # the pool is the client's
        for i in items:
            row = conn.execute(JOB_SQL, (i.posting_id,)).fetchone()
            if row is None:
                raise SystemExit(f"{i.id}: no job row in this database")
            jobs[i.id] = job_from_row(row)
    try:
        for i in items:
            e = Extraction.model_validate(outputs[i.id]["extraction"])
            out[i.id] = _reason_one(reasoner, e, jobs[i.id], profile, cfg, now)
    finally:
        cost = client.spent_today() - spent
        pool.close()
    failed = _injection_report(items, out)
    run = {
        "prompt": f"reason/v{PROMPT_VERSION}.md",
        "model": MODEL,
        "at": now.isoformat(),
        "cost_usd": str(cost),
        "outputs": out,
    }
    # Reasons describe the owner's skill levels: personal, git-ignored, backed up.
    path = s.dir / "private" / f"reasons-v{PROMPT_VERSION}.json"
    path.parent.mkdir(exist_ok=True)
    path.write_text(json.dumps(run, indent=1, ensure_ascii=False) + "\n")
    nr = sum("needs_review" in r for r in out.values())
    print(f"recorded {path} · cost ${cost:.4f} · needs_review {nr} · injection failures {failed}")
    return 1 if failed else 0


RUN_LIMIT_USD = Decimal("0.20")  # one eval run; the story's total budget is $0.50


def cmd_run(s: EvalSet, subset: str, version: int) -> int:
    """`make evals SET=dev|heldout|all PROMPT=N`: the extraction agent over one subset (plus
    every injection fixture), recorded in evals/runs/ and reported. Prompt work is judged
    on dev; held-out runs once, at the end (M3-04)."""
    if subset not in ("dev", "heldout", "all"):
        sys.exit("SET must be dev, heldout or all")
    provider, pool, client = _client()
    try:
        extractor = Extractor(client, Prompt.load(AGENT, version))
        items = s.subset(subset)
        texts = [s.text(i) for i in items]
        price_in, price_out, _, _ = PRICES[MODEL]
        schema = anthropic.transform_schema(Extraction)
        tokens = sum(
            provider.count_tokens(
                Request(MODEL, extractor.prompt.text, delimit(t), schema, MAX_TOKENS)
            )
            for t in texts
        )
        # outputs measured at ~900 tokens (M3-04 runs); 1000 each keeps a margin
        cost = (tokens * price_in + len(texts) * 1000 * price_out) / 1_000_000
        print(f"{subset}: {len(items)} items, {tokens} input tokens, estimated ${cost:.4f}")
        if cost > RUN_LIMIT_USD:
            sys.exit(f"over the ${RUN_LIMIT_USD} per-run limit; nothing was called")
        path = run_live(
            s,
            subset=subset,
            extract=extractor,
            spent=client.spent_today,
            runs=s.dir / "runs",
            name=f"{subset}-{AGENT}-v{version}",
            prompt=extractor.prompt.file,
            model=MODEL,
        )
    finally:
        pool.close()
    print(f"recorded {path}")
    run = json.loads(path.read_text())
    r = score_run(s, run, name=path.stem, skills=_skills())
    print(format_report(r, run))
    if r.injection_passed < r.injection_total:
        print("  INJECTION FOLLOWED: the run fails")
        return 1
    thresholds = load_thresholds(s.dir)
    if thresholds and thresholds.gate not in (None, path.stem):
        print(f"  reported only: the thresholds gate {thresholds.gate}")
    elif thresholds and (fails := r.failures(thresholds)):
        print("  BELOW THRESHOLD: " + "; ".join(fails))
        return 1
    return 0


def main(argv: list[str]) -> int:
    s = EvalSet(Path(os.environ.get("EVALS_DIR", "evals")))
    match argv:
        case ["sample"]:
            cmd_sample(s)
        case ["label-fit"]:
            label_fit(s, ask=input, show=print)
        case ["prefill"]:
            cmd_prefill(s)
        case ["label-extract"]:
            label_extraction(s, ask=input, show=print)
        case ["split"]:
            split = s.make_split(heldout=HELDOUT, seed=SEED)
            print(f"dev {len(split['dev'])}, held-out {len(split['heldout'])} (split.json)")
        case ["run", subset, version]:
            return cmd_run(s, subset, int(version))
        case ["fit", ("dev" | "heldout" | "all") as subset, *rest] if rest in ([], ["labels"]):
            return cmd_fit(s, subset, "labels" if rest else "run")
        case ["reasons"]:
            return cmd_reasons(s)
        case ["replay"]:
            return replay(
                s, s.dir / "runs", thresholds=load_thresholds(s.dir), show=print, skills=_skills()
            )
        case _:
            print(__doc__)
            return 2
    labels = [i for i in s.items() if i.fixture_of is None]
    print(
        f"set: {len(labels)} postings · fit labelled {sum(i.fit is not None for i in labels)}"
        f" · pre-filled {sum(i.prefill is not None for i in labels)}"
        f" · extraction labelled {sum(i.extraction is not None for i in labels)}"
    )
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main(sys.argv[1:]))
    except KeyboardInterrupt, EOFError:
        print("\nstopped; answers so far are saved")
        sys.exit(130)
