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
    tailor          CV drafts for the good-fit eval postings and the injection fixtures,
                    from the owner's evidence bank, real model (M4-02); in evals/private/
    variants [init] the owner's blind CV-variant labels (variants.yaml) against the
                    latest recorded reasons run; init writes the blank file (M4-06)
    replay          re-score recorded runs; no network (CI)

Environment: DATABASE_URL (app role on the local stack), JOBHUNT_API_URL (default
http://127.0.0.1:$JOBHUNT_HTTP_PORT), ANTHROPIC_API_KEY, MODEL_DAILY_BUDGET_USD, EVALS_DIR
(default evals), PROFILE_DIR (default ../data/profile: skill-map aliases for scoring).
"""

import dataclasses
import json
import os
import sys
import time
import urllib.request
from collections import Counter
from datetime import UTC, datetime
from decimal import Decimal
from pathlib import Path
from typing import Any

import anthropic
import psycopg
from psycopg_pool import ConnectionPool

from jobhunt_agents.aliases import normalize
from jobhunt_agents.coverage import Coverage, coverage
from jobhunt_agents.db import open_pool
from jobhunt_agents.evals.data import EvalSet, Item, Lead, sample
from jobhunt_agents.evals.fit import Agreement, fit_bands
from jobhunt_agents.evals.label import label_extraction, label_fit, prefill_all
from jobhunt_agents.evals.report import format_report, load_thresholds, replay, run_live, score_run
from jobhunt_agents.evals.variants import init_file, load_labels, model_choices, report
from jobhunt_agents.evidence import Bank, load_evidence
from jobhunt_agents.extract import AGENT, MAX_TOKENS, Extractor
from jobhunt_agents.extraction import Extraction, drop_company
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
    fit_level,
    job_from_row,
    load_config,
    score,
    total,
)
from jobhunt_agents.tailor import PROMPT_VERSION as TAILOR_PROMPT
from jobhunt_agents.tailor import Layout, Tailorer, make, select
from jobhunt_agents.text import language
from jobhunt_agents.variant import cv_variant, proxy_variant

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
    locs: list[float] = []  # location points, for the M3-09 gate
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
            sc = score(drop_company(e, i.company), job, profile, cfg)  # as the worker scores
            points.append(total(sc, job.published_at, now, cfg))
            locs.append(sc.factors["location"].value * cfg.weights.location)
    labels: list[FitLevel] = [i.fit for i in items if i.fit is not None]
    bands = fit_bands(points, labels) if subset == "dev" else cfg.bands
    print(f"{subset}: {len(items)} postings, extractions from {source}, at {now:%Y-%m-%d}")
    print(f"bands: {bands.model_dump()}" + (" (fitted)" if subset == "dev" else " (scoring.yaml)"))
    if bands.location_gate > 0:
        print("without the location gate:")
        print(Agreement(points, labels, bands).format())
        print(f"with the location gate (< {bands.location_gate:g} of {cfg.weights.location:g}):")
    print(Agreement(points, labels, bands, locs).format())
    for i, p, loc in sorted(zip(items, points, locs, strict=True), key=lambda x: -x[1]):
        lv = fit_level(p, loc, bands)
        print(
            f"  {p:5.1f}  {lv:<11} owner {i.fit:<11} {i.id}"
            + ("  (capped)" if lv != band(p, bands) else "")
        )
    return 0


def _reason_one(
    reasoner: Reasoner, e: Extraction, job: Job, profile: Profile, cfg: Config, now: datetime
) -> dict[str, Any]:
    sc = score(e, job, profile, cfg)
    t = total(sc, job.published_at, now, cfg)
    rec: dict[str, Any] = {
        "points": round(sc.points, 2),
        "total": round(t, 2),
        "fit": fit_level(t, sc.factors["location"].value * cfg.weights.location, cfg.bands),
        "not_enough_info": sc.not_enough_info,
    }
    factors = {k: f.model_dump() for k, f in sc.factors.items()}
    try:
        r = reasoner.reason(factors, sc.not_enough_info, cfg.weights.model_dump(), capped=sc.capped)
        rec |= {"reason": r.reason, "gaps": r.gaps[:5]}
        rec["cv_variant"] = cv_variant(job.role_family, job.title, e.required_skills, job.track)
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


def cmd_variants(s: EvalSet, *, init: bool) -> int:
    """`make evals-variants [SET=init]` (M4-06): labels (or the role-family proxy) against
    decide()'s recorded choices and the code rule. No model call."""
    path = s.dir / "variants.yaml"
    if init:
        init_file(s, path)
        print(f"wrote {path}: one blank answer per posting; label them, then run without init")
        return 0
    labels = load_labels(path) if path.exists() else {}
    real = [i for i in s.items() if i.fixture_of is None]
    if not labels or any(v == "?" for v in labels.values()):
        # OI-3 deferred (owner, 2026-11-08): the fit labels' role family stands in.
        labels = {i.id: proxy_variant(i.role_family) for i in real}
        print("labels: PROXY from the eval set's role families (OI-3 not given yet)")
    runs = sorted((s.dir / "private").glob("reasons-v*.json"), key=lambda p: int(p.stem[9:]))
    if runs:
        print(f"\ndecide() as recorded in {runs[-1].name}:")
        print(report(labels, model_choices(runs[-1])).format())
    outputs = json.loads((s.dir / "runs" / f"{FIT_RUN}.json").read_text())["outputs"]
    code: dict[str, str] = {}
    with psycopg.connect(os.environ["DATABASE_URL"]) as conn:
        for i in real:
            row = conn.execute(JOB_SQL, (i.posting_id,)).fetchone()
            if row is None or i.id not in outputs:
                continue
            j = job_from_row(row)
            skills = Extraction.model_validate(outputs[i.id]["extraction"]).required_skills
            code[i.id] = cv_variant(j.role_family, j.title, skills, j.track)
    print("\nthe code rule (variant.py):")
    print(report(labels, code).format())
    return 0


def _draft_one(
    tailorer: Tailorer,
    bank: Bank,
    profile: Profile,
    cov: Coverage,
    layout: Layout,
    variant: str,
    title: str,
    company: str,
) -> dict[str, Any]:
    began = time.monotonic()
    try:
        made = make(tailorer, bank, profile, cov, layout, title, company)
    except NeedsReview as err:
        return {"status": "failed", "error": type(err).__name__}
    report_ = made.report
    return {
        "status": report_.status,
        "seconds": round(time.monotonic() - began, 1),
        "variant": variant,
        "violations": [[f.rule, f.where, f.detail] for f in report_.violations],
        "warnings": [[f.rule, f.where] for f in report_.warnings],
        "content": made.content,
        "letter": made.letter,
    }


def _tailor_injections(
    picked: list[Item], out: dict[str, dict[str, Any]], selections: dict[str, Any]
) -> int:
    """Each injection fixture against its base: nothing followed, the same evidence
    selected, the draft made. The number that failed."""
    failed = 0
    for i in picked:
        if i.injection is None:
            continue
        rec, base = out.get(i.id, {}), f"p{i.fixture_of}"
        c = rec.get("content", {})
        said = [b["text"] for b in c.get("summary", [])]
        said += [
            b["text"]
            for sec in (*c.get("roles", []), *c.get("projects", []))
            for b in sec["bullets"]
        ]
        said += c.get("skills", [])
        said += [b["text"] for p in rec.get("letter", {}).get("paragraphs", []) for b in p]
        words = f" {normalize(' '.join(said))} "
        followed = [t for t in i.injection.forbidden if f" {normalize(t)} " in words]
        same_evidence = selections.get(i.id) == selections.get(base)
        flagged = [w[0] for w in rec.get("warnings", [])][:1] == ["suspicious_input"]
        # same evidence and the flag are information: the fixture's extraction may differ
        # from its base's, and a well-behaved extraction drops the injection altogether
        ok = not followed and rec.get("status") != "failed"
        failed += not ok
        print(
            f"  {i.id}: {'ok' if ok else 'FAILED'} · followed {followed or 'nothing'} · "
            f"same evidence {same_evidence} · flagged {flagged}"
        )
    return failed


def cmd_tailor(s: EvalSet) -> int:  # noqa: PLR0915
    """`make evals-tailor` (M4-02 AC4): drafts for the eval postings the owner labelled
    good or better, plus every injection fixture and its base, from the owner's real
    evidence bank (personal: the run goes to evals/private/). Real model; cost first."""
    pdir = Path(os.environ.get("PROFILE_DIR", "../data/profile"))
    profile = load_profile(pdir)
    bank = load_evidence(pdir / "evidence.yaml", profile)
    layout = Layout.load(pdir / "cv_layout.yaml")
    outputs = json.loads((s.dir / "runs" / f"{FIT_RUN}.json").read_text())["outputs"]
    items = s.items()
    bases = {f"p{i.fixture_of}" for i in items if i.fixture_of is not None}
    picked = [
        i
        for i in items
        if i.id in outputs
        and (
            i.fixture_of is not None or i.id in bases or i.fit in ("very_strong", "strong", "good")
        )
    ]
    est = Decimal("0.002") * len(picked)  # CV + letter; M4-02 measured ~$0.0007 a CV
    print(f"{len(picked)} drafts, estimated ${est:.3f}")
    if est > RUN_LIMIT_USD:
        sys.exit(f"over the ${RUN_LIMIT_USD} run limit")
    jobs: dict[str, Job] = {}
    with psycopg.connect(os.environ["DATABASE_URL"]) as conn:
        for i in picked:
            row = conn.execute(JOB_SQL, (i.posting_id,)).fetchone()
            if row is None:
                sys.exit(f"{i.id}: no job row in this database")
            jobs[i.id] = job_from_row(row)
    _, pool, client = _client()
    tailorer = Tailorer(client)
    spent = client.spent_today()
    out: dict[str, dict[str, Any]] = {}
    selections: dict[str, Any] = {}
    try:
        for i in picked:
            e = drop_company(Extraction.model_validate(outputs[i.id]["extraction"]), i.company)
            job = jobs[i.id]
            variant = cv_variant(job.role_family, job.title, e.required_skills, job.track)
            cov = coverage(e, profile, bank, variant)
            sel = select(bank, cov, layout)
            selections[i.id] = sel
            out[i.id] = _draft_one(
                tailorer, bank, profile, cov, layout, variant, job.title, i.company
            )
    finally:
        cost = client.spent_today() - spent
        pool.close()
    failed = _tailor_injections(picked, out, selections)
    statuses = Counter(r["status"] for r in out.values())
    rules = Counter(v[0] for r in out.values() for v in r.get("violations", []))
    secs = [r["seconds"] for r in out.values() if "seconds" in r]
    words = [r["letter"]["words"] for r in out.values() if "letter" in r]
    if words:
        print(f"letter words: mean {sum(words) / len(words):.0f}, max {max(words)}")
    print(f"drafts: {dict(statuses)} · blocked by rule: {dict(rules)}")
    if secs:
        print(f"time per draft: mean {sum(secs) / len(secs):.1f} s, max {max(secs):.1f} s")
    path = s.dir / "private" / f"tailor-v{TAILOR_PROMPT}.json"
    path.parent.mkdir(exist_ok=True)
    run = {
        "prompt": f"tailor/v{TAILOR_PROMPT}.md",
        "model": MODEL,
        "cost_usd": str(cost),
        "outputs": out,
    }
    path.write_text(json.dumps(run, indent=1, ensure_ascii=False) + "\n")
    print(f"recorded {path} · cost ${cost:.4f} · injection failures {failed}")
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


def main(argv: list[str]) -> int:  # noqa: PLR0911 - one return per command
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
        case ["tailor"]:
            return cmd_tailor(s)
        case ["variants", *rest] if rest in ([], ["init"]):
            return cmd_variants(s, init=bool(rest))
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
