"""Coded scores for shown leads (M3-06 AC1, AC4, ADR-0021), and the lazy queueing of
their reasons (AC5). No model call here.

Every worker iteration runs `rescore`. It recomputes only scores whose inputs changed:
the extraction, weights, bands, profile or reason prompt. A change of scoring.yaml or the
profile files therefore reaches every shown lead in one pass.
"""

import hashlib
import json

from psycopg.types.json import Jsonb
from psycopg_pool import ConnectionPool

from jobhunt_agents.extraction import Extraction, drop_company
from jobhunt_agents.profile import Profile
from jobhunt_agents.reason import PROMPT_VERSION as REASON_PROMPT_VERSION
from jobhunt_agents.score import Config, Scored, job_from_row, score

# "Shown" = has an extract item: Go queues those only for leads the default preset shows.
# ponytail: a lead hidden by a later preset change keeps its score (ADR-0021); job facet
# changes alone (a new classifier) don't trigger a rescore until another input changes.
_STALE = """
SELECT p.id, p.content_hash, e.id, e.prompt_version, e.data, c.name,
       j.title, j.seniority, j.experience_min_years, j.workplace, j.remote_scopes,
       j.countries, j.locations, j.published_at, j.matched_location
FROM work_items w
JOIN postings p ON p.id = w.posting_id
JOIN jobs j ON j.posting_id = p.id
JOIN companies c ON c.id = j.company_id
JOIN LATERAL (
    SELECT x.id, x.prompt_version, x.data FROM extractions x
    WHERE x.posting_id = p.id AND x.content_hash = p.content_hash
    ORDER BY x.prompt_version DESC LIMIT 1
) e ON true
LEFT JOIN scores s ON s.posting_id = p.id
WHERE w.kind = 'extract'
  AND (s.posting_id IS NULL OR s.extraction_id <> e.id
       OR s.weights_version <> %(weights)s OR s.bands_version <> %(bands)s
       OR s.profile_version <> %(profile)s OR NOT starts_with(s.reason_inputs, %(prefix)s))
"""

_UPSERT = """
INSERT INTO scores (posting_id, content_hash, extraction_id, extraction_prompt_version,
    points, factors, not_enough_info, freshness, bands, weights_version, bands_version,
    profile_version, reason_inputs)
VALUES (%(posting)s, %(hash)s, %(extraction)s, %(prompt)s, %(points)s, %(factors)s,
    %(nei)s, %(freshness)s, %(bands_json)s, %(weights)s, %(bands)s, %(profile)s, %(inputs)s)
ON CONFLICT (posting_id) DO UPDATE SET
    content_hash = excluded.content_hash, extraction_id = excluded.extraction_id,
    extraction_prompt_version = excluded.extraction_prompt_version,
    points = excluded.points, factors = excluded.factors,
    not_enough_info = excluded.not_enough_info, freshness = excluded.freshness,
    bands = excluded.bands, weights_version = excluded.weights_version,
    bands_version = excluded.bands_version, profile_version = excluded.profile_version,
    reason_inputs = excluded.reason_inputs, scored_at = now()
"""

# A reason is queued at once for posting content it hasn't described (a new or changed
# posting), otherwise only when the lead was opened after the reason was written and its
# inputs changed since (AC5). A failed (needs_review) item waits for `queue retry`.
_QUEUE_NEW_CONTENT = """
INSERT INTO work_items (posting_id, kind, content_hash)
SELECT s.posting_id, 'reason', s.content_hash
FROM scores s LEFT JOIN score_reasons r ON r.posting_id = s.posting_id
WHERE r.posting_id IS NULL OR r.content_hash <> s.content_hash
ON CONFLICT (posting_id, kind) DO UPDATE SET
    state = 'pending', attempts = 0, last_error = NULL, run_after = now(),
    content_hash = excluded.content_hash, updated_at = now()
WHERE work_items.state <> 'running' AND work_items.content_hash <> excluded.content_hash
"""

_QUEUE_REQUESTED = """
UPDATE work_items w
SET state = 'pending', attempts = 0, last_error = NULL, run_after = now(), updated_at = now()
FROM scores s JOIN score_reasons r ON r.posting_id = s.posting_id
WHERE w.posting_id = s.posting_id AND w.kind = 'reason' AND w.state = 'done'
  AND s.reason_requested_at > r.created_at AND r.inputs <> s.reason_inputs
"""


def reason_inputs(s: Scored) -> str:
    """What a reason is made from: the breakdown, the profile, the location cap (M3-09)
    and the reason prompt."""
    factors = json.dumps({k: f.model_dump() for k, f in s.factors.items()}, sort_keys=True)
    h = hashlib.sha256(f"{factors}\0{s.profile_version}\0{s.capped}".encode()).hexdigest()
    return f"v{REASON_PROMPT_VERSION}:{h}"


def rescore(pool: ConnectionPool, profile: Profile, cfg: Config) -> int:
    """Scores the shown leads whose inputs changed and queues their reasons; the number
    of scores written."""
    keys = {
        "weights": cfg.weights_version,
        "bands": cfg.bands_version,
        "profile": profile.version,
        "prefix": f"v{REASON_PROMPT_VERSION}:",
    }
    freshness = Jsonb({"weight": cfg.weights.freshness, **cfg.freshness.model_dump()})
    bands = Jsonb(cfg.bands.model_dump())
    w = cfg.weights.model_dump()
    n = 0
    with pool.connection() as conn:
        for posting, content_hash, ext_id, prompt, data, company, *job in conn.execute(
            _STALE, keys
        ).fetchall():
            e = drop_company(Extraction.model_validate(data), company)  # stored before the filter
            s = score(e, job_from_row(tuple(job)), profile, cfg)
            conn.execute(
                _UPSERT,
                {
                    **keys,
                    "posting": posting,
                    "hash": content_hash,
                    "extraction": ext_id,
                    "prompt": prompt,
                    "points": s.points,
                    "factors": Jsonb(
                        {k: f.model_dump() | {"weight": w[k]} for k, f in s.factors.items()}
                    ),
                    "nei": s.not_enough_info,
                    "freshness": freshness,
                    "bands_json": bands,
                    "inputs": reason_inputs(s),
                },
            )
            n += 1
        conn.execute(_QUEUE_NEW_CONTENT)
        conn.execute(_QUEUE_REQUESTED)
    return n
