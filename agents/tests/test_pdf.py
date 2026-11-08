"""M5-03: an approved draft as two A4 one-page PDFs from the stored text only; a long CV
loses its lowest-ranked bullets, never part of a sentence; the text layer is real and
keeps accents (ATS); markup prints as text and nothing is active; stored as documents."""

import copy
import hashlib
import io
import json
import re
from pathlib import Path
from typing import Any

import psycopg
import pytest
from pypdf import PdfReader

from jobhunt_agents.db import open_pool
from jobhunt_agents.pdf import RENDERER_VERSION, ExportError, cv_pdf, export_approved, letter_pdf
from tests.conftest import Go
from tests.test_tailor import latest, lead, ok, run

PERSON = {
    "name": "Zoë Côté",
    "headline": "Backend Engineer",
    "email": "zoe@example.com",
    "phone": "",
    "location": "Abidjan, Côte d'Ivoire",
    "links": ["https://github.com/example"],
}
CONTENT: dict[str, Any] = {
    "person": PERSON,
    "summary": [
        {"text": "Backend engineer with 3 years building Node.js services.", "evidence_ids": ["s"]}
    ],
    "roles": [
        {
            "id": "r",
            "employer": "Société Générale",
            "title": "Backend Engineer",
            "start": "2023-05",
            "end": "present",
            "location": "Abidjan",
            "bullets": [
                {
                    "text": "Cut the invoice job's run time from 40 to 6 minutes.",
                    "evidence_ids": ["a"],
                },
                {"text": "Moved the services to Docker images built in CI.", "evidence_ids": ["b"]},
            ],
        }
    ],
    "projects": [],
    "skills": ["PostgreSQL", "Node.js"],
    "education": [
        {
            "id": "e",
            "school": "École Polytechnique",
            "degree": "Diplôme d'ingénieur",
            "location": "",
            "start": "2019",
            "end": "2022",
        }
    ],
    "certifications": [],
    "languages": [{"name": "Français", "level": "natif"}, {"name": "Ελληνικά", "level": "A2"}],
}
LETTER = {
    "greeting": "Dear Hiring Team,",
    "opening": "I am writing to apply for the Backend Engineer role at Ünïcode GmbH.",
    "paragraphs": [
        [{"text": "I cut the invoice job from 40 to 6 minutes.", "evidence_ids": ["a"]}]
    ],
    "closing": "Best regards,",
    "name": "Zoë Côté",
    "words": 30,
}


def text_of(pdf: bytes) -> tuple[int, str]:
    r = PdfReader(io.BytesIO(pdf))
    return len(r.pages), " ".join((p.extract_text() or "") for p in r.pages)


def flat(s: str) -> str:
    return " ".join(s.split())


# AC1 + AC3: every stored sentence and fact is in the text layer, accents and non-Latin
# names intact, on one A4 page.
def test_cv_text_layer_keeps_everything() -> None:
    pages, text = text_of(cv_pdf(CONTENT))
    assert pages == 1
    t = flat(text)
    for want in (
        "Zoë Côté",
        "Côte d'Ivoire",
        "Société Générale",
        "École Polytechnique",
        "Ελληνικά",
        "Cut the invoice job's run time from 40 to 6 minutes.",
        "Moved the services to Docker images built in CI.",
        "2023-05 \u2013 present",
    ):
        assert want in t, want
    r = PdfReader(io.BytesIO(cv_pdf(CONTENT)))
    assert abs(float(r.pages[0].mediabox.width) - 595.28) < 1  # A4


def test_letter_text_layer() -> None:
    pages, text = text_of(letter_pdf(LETTER, PERSON))
    assert pages == 1
    t = flat(text)
    for want in (
        "Dear Hiring Team,",
        "Ünïcode GmbH",
        "I cut the invoice job from 40 to 6 minutes.",
        "Best regards,",
        "Zoë Côté",
    ):
        assert want in t, want


# AC2: a CV over one page loses the lowest-ranked bullets (the last of the longest
# section) until it fits; every kept sentence is whole.
def test_long_cv_drops_lowest_ranked_bullets() -> None:
    c = copy.deepcopy(CONTENT)
    c["roles"][0]["bullets"] = [
        {
            "text": f"Bullet {i:02d}: " + "rebuilt a service and measured the result " * 4 + "end.",
            "evidence_ids": ["a"],
        }
        for i in range(60)
    ]
    pages, text = text_of(cv_pdf(c))
    t = flat(text)
    assert pages == 1
    assert "Bullet 00:" in t  # the most relevant stays
    assert "Bullet 59:" not in t  # the least relevant goes first
    kept = [i for i in range(60) if f"Bullet {i:02d}:" in t]
    assert kept == list(range(len(kept)))  # dropped from the end only
    assert t.count("end.") == len(kept)  # no sentence cut
    assert len(c["roles"][0]["bullets"]) == 60  # the stored draft is not changed


def test_cv_that_cannot_fit_fails() -> None:
    c = copy.deepcopy(CONTENT)
    c["roles"] = [
        {
            **CONTENT["roles"][0],
            "id": f"r{i}",
            "bullets": [{"text": "word " * 120, "evidence_ids": ["a"]}],
        }
        for i in range(12)
    ]
    with pytest.raises(ExportError):
        cv_pdf(c)


# AC4: markup and control characters print as literal text; the PDF has no links,
# scripts, annotations or embedded files.
def test_markup_is_inert() -> None:
    c = copy.deepcopy(CONTENT)
    payload = "<script>alert(1)</script> **bold** [x](javascript:alert(1))"
    c["roles"][0]["bullets"][0]["text"] = payload + " tail\x07bell\x1b[31m"
    pdf = cv_pdf(c)
    t = flat(text_of(pdf)[1])
    assert payload in t
    assert "tail bell [31m" in t  # control characters become spaces, words stay apart
    for active in (
        b"/JavaScript",
        b"/JS",
        b"/URI",
        b"/Annots",
        b"/EmbeddedFile",
        b"/Launch",
    ):
        assert active not in pdf, active
    # fpdf2's /OpenAction is the initial view (a page and zoom array), never an action
    assert re.findall(rb"/OpenAction\s*(\S)", pdf) in ([], [b"["])


# AC1, M5-04: an approved draft is exported once into two documents (bytes, hash,
# versions); a draft that can't be exported records why and stores nothing.
def test_approved_draft_exported_once(app_dsn: str, go: Go, profile_dir: Path) -> None:
    posting = lead(app_dsn, go, profile_dir)
    run(
        app_dsn,
        profile_dir,
        __import__("tests.fakes", fromlist=["FakeProvider"]).FakeProvider(*ok()),
    )
    d = latest(app_dsn, posting)
    assert d["status"] == "ready_for_review", d["findings"]
    with psycopg.connect(app_dsn) as conn:
        conn.execute("UPDATE drafts SET status = 'approved' WHERE id = %s", (d["id"],))
    with open_pool(app_dsn, max_size=2) as pool:
        assert export_approved(pool) >= 1
        assert export_approved(pool) == 0  # once
    with psycopg.connect(app_dsn) as conn:
        rows = conn.execute(
            "SELECT kind, pdf, sha256, evidence_version, renderer_version FROM documents"
            " WHERE draft_id = %s ORDER BY kind",
            (d["id"],),
        ).fetchall()
    assert [r[0] for r in rows] == ["cv", "letter"]
    for _, pdf, sha, ev, rv in rows:
        assert hashlib.sha256(pdf).digest() == sha
        assert ev == d["evidence_version"]
        assert rv == RENDERER_VERSION
        assert "Sam Example" in text_of(pdf)[1]


def test_unexportable_draft_records_why(app_dsn: str, go: Go, profile_dir: Path) -> None:
    posting = lead(app_dsn, go, profile_dir)
    big = copy.deepcopy(CONTENT)
    big["roles"] = [
        {
            **CONTENT["roles"][0],
            "id": f"r{i}",
            "bullets": [{"text": "word " * 120, "evidence_ids": ["a"]}],
        }
        for i in range(12)
    ]
    with psycopg.connect(app_dsn) as conn:
        # lead() asks for a draft; drop that request so no tailor item is left for others
        conn.execute(
            "UPDATE drafts SET status = 'discarded' WHERE posting_id = %s AND status = 'requested'",
            (posting,),
        )
        draft = conn.execute(
            "INSERT INTO drafts (posting_id, status, content, evidence_version)"
            " VALUES (%s, 'approved', %s, 'v') RETURNING id",
            (posting, json.dumps(big)),
        ).fetchone()
    assert draft
    with open_pool(app_dsn, max_size=2) as pool:
        export_approved(pool)
        assert export_approved(pool) == 0  # not retried
    with psycopg.connect(app_dsn) as conn:
        err = conn.execute("SELECT error FROM drafts WHERE id = %s", draft).fetchone()
        n = conn.execute("SELECT count(*) FROM documents WHERE draft_id = %s", draft).fetchone()
    assert err
    assert err[0].startswith("export:")
    assert n == (0,)
