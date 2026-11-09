"""PDF export (M5-03, ADR-0027): an approved draft as two A4 one-page PDFs, CV and letter,
from the stored text only (no model call, nothing the draft doesn't hold), then stored
as immutable documents (M5-04, ADR-0026).

Text only: no links, scripts, annotations or embedded files; markup in the text prints
as it is. A vendored OFL font (fonts/) so accents and non-Latin names render, and real
text for ATS software. A CV over one page loses its lowest-ranked bullets (each section
keeps its most relevant first), never part of a sentence.
"""

import copy
import hashlib
import logging
import unicodedata
from pathlib import Path
from typing import Any

from fpdf import FPDF
from psycopg_pool import ConnectionPool

log = logging.getLogger(__name__)

RENDERER_VERSION = 3  # 2: left-aligned, no blank lines; 3: French headings (M7-02)
FONTS = Path(__file__).parent / "fonts"
MARGIN = 15  # mm
BODY = 9.5  # pt


class ExportError(Exception):
    """The draft can't be exported (it doesn't fit one page even at one bullet a section)."""


def clean(text: str) -> str:
    """Printable text: control characters become spaces (nothing is interpreted)."""
    return "".join(" " if unicodedata.category(c) == "Cc" else c for c in str(text))


class _Doc(FPDF):
    def __init__(self) -> None:
        super().__init__(format="A4")
        self.set_margins(MARGIN, MARGIN, MARGIN)
        self.set_auto_page_break(auto=True, margin=MARGIN)
        self.add_font("Noto", "", FONTS / "NotoSans-Regular.ttf")
        self.add_font("Noto", "B", FONTS / "NotoSans-Bold.ttf")
        self.set_creator("job-hunt-agent")
        self.add_page()

    def para(self, value: str, size: float = BODY, style: str = "", gap: float = 1) -> None:
        if not value.strip():  # an empty line (no dates, say) takes no space
            return
        self.set_font("Noto", style, size)
        self.multi_cell(0, size * 0.45, clean(value), align="L", new_x="LMARGIN", new_y="NEXT")
        self.ln(gap)

    def heading(self, value: str) -> None:
        self.ln(1.5)
        self.para(value.upper(), 10, "B", 0.5)
        self.line(self.l_margin, self.get_y(), self.w - self.r_margin, self.get_y())
        self.ln(1.5)

    def bullet(self, value: str) -> None:
        self.set_font("Noto", "", BODY)
        self.cell(4, BODY * 0.45, "•")
        self.multi_cell(0, BODY * 0.45, clean(value), align="L", new_x="LMARGIN", new_y="NEXT")
        self.ln(0.6)


# Headings by the draft's language (M7-02); English when none is stored.
_LABELS = {
    "fr": {
        "Summary": "Profil",
        "Experience": "Expérience",
        "Projects": "Projets",
        "Skills": "Compétences",
        "Education": "Formation",
        "Certifications": "Certifications",
        "Languages": "Langues",
        "present": "aujourd\u2019hui",
    }
}


def _dates(x: dict[str, Any], t: dict[str, str] | None = None) -> str:
    t = t or {}
    parts = (x.get("start"), x.get("end"))
    return " \u2013 ".join(t.get(str(v), str(v)) for v in parts if v)  # en dash


def _cv(c: dict[str, Any]) -> _Doc:
    d = _Doc()
    t = _LABELS.get(c.get("language", "en"), {})

    def tr(s: str) -> str:
        return t.get(s, s)

    p = c["person"]
    d.para(p["name"], 18, "B", 0.5)
    d.para(" · ".join(v for v in (p.get("headline"), p.get("location")) if v), BODY, "", 0)
    d.para(" · ".join(v for v in (p.get("email"), p.get("phone"), *p.get("links", [])) if v))
    if c["summary"]:
        d.heading(tr("Summary"))
        d.para(" ".join(b["text"] for b in c["summary"]))
    for kind, title in (("roles", "Experience"), ("projects", "Projects")):
        if not c[kind]:
            continue
        d.heading(tr(title))
        for sec in c[kind]:
            head = " · ".join(
                v
                for v in (
                    sec.get("title") or sec.get("name"),
                    sec.get("employer"),
                    sec.get("stack"),
                )
                if v
            )
            d.para(head, BODY + 0.5, "B", 0)
            d.para(
                " · ".join(v for v in (sec.get("location"), _dates(sec, t)) if v), BODY - 1, "", 0.8
            )
            for b in sec["bullets"]:
                d.bullet(b["text"])
            d.ln(1)
    if c["skills"]:
        d.heading(tr("Skills"))
        d.para(" · ".join(c["skills"]))
    if c.get("education"):
        d.heading(tr("Education"))
        for e in c["education"]:
            d.para(f"{e['degree']} · {e['school']}", BODY, "B", 0)
            d.para(" · ".join(v for v in (e.get("location"), _dates(e, t)) if v), BODY - 1)
    if c.get("certifications"):
        d.heading(tr("Certifications"))
        for x in c["certifications"]:
            d.para(f"{x['name']} · {x['issuer']} · {x.get('date', '')}")
    if c.get("languages"):
        d.heading(tr("Languages"))
        d.para(" · ".join(f"{x['name']} ({x['level']})" for x in c["languages"]))
    return d


def _drop_one(c: dict[str, Any]) -> bool:
    """Drops the last (lowest-ranked) bullet of the longest section; False if every
    section is down to one."""
    secs = [s for k in ("roles", "projects") for s in c[k] if len(s["bullets"]) > 1]
    if not secs:
        return False
    max(secs, key=lambda s: len(s["bullets"]))["bullets"].pop()
    return True


def cv_pdf(content: dict[str, Any]) -> bytes:
    c = copy.deepcopy(content)
    while True:
        d = _cv(c)
        if d.page_no() == 1:
            return bytes(d.output())
        if not _drop_one(c):
            raise ExportError("the CV doesn't fit one page, even with one bullet a section")


def letter_pdf(letter: dict[str, Any], person: dict[str, Any]) -> bytes:
    d = _Doc()
    d.para(person["name"], 14, "B", 0.5)
    d.para(
        " · ".join(
            v for v in (person.get("email"), person.get("phone"), person.get("location")) if v
        ),
        BODY,
        "",
        6,
    )
    d.para(letter["greeting"], 10.5, "", 3)
    d.para(letter["opening"], 10.5, "", 3)
    for para in letter["paragraphs"]:
        d.para(" ".join(b["text"] for b in para), 10.5, "", 3)
    d.para(letter["closing"], 10.5, "", 1)
    d.para(letter["name"], 10.5)
    if d.page_no() != 1:
        raise ExportError("the letter doesn't fit one page")
    return bytes(d.output())


_APPROVED = """
SELECT d.id, d.posting_id, d.content, d.letter, d.evidence_version
FROM drafts d
WHERE d.status = 'approved' AND d.error IS NULL
  AND NOT EXISTS (SELECT 1 FROM documents x WHERE x.draft_id = d.id)
ORDER BY d.id
"""

_INSERT = """
INSERT INTO documents (draft_id, posting_id, kind, pdf, sha256, evidence_version, renderer_version)
VALUES (%s, %s, %s, %s, %s, %s, %s)
"""

_ERROR = "UPDATE drafts SET error = %s WHERE id = %s"


def export_approved(pool: ConnectionPool) -> int:
    """Exports every approved draft that has no documents yet; how many it exported. A
    draft that can't be exported records why (shown on the page) and is not retried."""
    with pool.connection() as conn:
        rows = conn.execute(_APPROVED).fetchall()
    done = 0
    for draft_id, posting_id, content, letter, evidence_version in rows:
        try:
            pdfs = [("cv", cv_pdf(content))]
            if letter:
                pdfs.append(("letter", letter_pdf(letter, content["person"])))
        except ExportError as e:
            with pool.connection() as conn:
                conn.execute(_ERROR, (f"export: {e}", draft_id))
            log.warning("export failed", extra={"draft_id": draft_id})
            continue
        with pool.connection() as conn, conn.transaction():
            for kind, pdf in pdfs:
                conn.execute(
                    _INSERT,
                    (
                        draft_id,
                        posting_id,
                        kind,
                        pdf,
                        hashlib.sha256(pdf).digest(),
                        evidence_version,
                        RENDERER_VERSION,
                    ),
                )
        log.info("draft exported", extra={"draft_id": draft_id, "documents": len(pdfs)})
        done += 1
    return done
