"""Posting text for prompts and for the terminal (M3-07; M3-04 AC6 builds on it).

Postings are untrusted: HTML is parsed (never rendered), script and style content is
dropped, and text shown in a terminal loses its control characters.
"""

import html
import re
import unicodedata
from html.parser import HTMLParser
from typing import Any, cast

_BLOCK = {"p", "div", "br", "li", "ul", "ol", "h1", "h2", "h3", "h4", "h5", "h6", "tr", "section"}


class _Text(HTMLParser):
    def __init__(self) -> None:
        super().__init__(convert_charrefs=True)
        self.parts: list[str] = []
        self._skip = 0

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        if tag in ("script", "style"):
            self._skip += 1
        elif tag in _BLOCK:
            self.parts.append("\n")
            if tag == "li":
                self.parts.append("- ")

    def handle_endtag(self, tag: str) -> None:
        if tag in ("script", "style"):
            self._skip = max(0, self._skip - 1)
        elif tag in _BLOCK:
            self.parts.append("\n")

    def handle_data(self, data: str) -> None:
        if not self._skip:
            self.parts.append(data)


def clean_html(s: str) -> str:
    """HTML (Greenhouse sends it entity-escaped) to plain lines and bullets."""
    p = _Text()
    p.feed(html.unescape(s))
    p.close()
    lines = (" ".join(line.split()) for line in "".join(p.parts).splitlines())
    return "\n".join(line for line in lines if line)


def _str(v: object) -> str:
    return v if isinstance(v, str) else ""


def posting_text(source: str, payload: dict[str, Any], *, title: str, location: str) -> str:
    """The text an agent reads: title, location, then the description, per board."""
    body: list[str] = []
    if source == "lever":
        body.append(
            _str(payload.get("descriptionPlain")) or clean_html(_str(payload.get("description")))
        )
        lists: list[object] = payload.get("lists") or []
        for entry in lists:
            if isinstance(entry, dict):
                item = cast(dict[str, object], entry)
                body.append(_str(item.get("text")))
                body.append(clean_html(_str(item.get("content"))))
        body.append(
            _str(payload.get("additionalPlain")) or clean_html(_str(payload.get("additional")))
        )
    elif source == "greenhouse":
        body.append(clean_html(_str(payload.get("content"))))
    elif source == "ashby":
        body.append(
            _str(payload.get("descriptionPlain"))
            or clean_html(_str(payload.get("descriptionHtml")))
        )
    else:  # manual postings and anything new: whatever description there is
        body.append(clean_html(_str(payload.get("description"))))
    text = "\n".join(b.strip() for b in body if b.strip())
    return f"Title: {title}\nLocation: {location}\n\n{text}"


def terminal_safe(s: str) -> str:
    """Drops control characters (escape sequences, bells, carriage returns) but keeps
    newlines and tabs, so untrusted text can't drive the terminal."""
    return "".join(c for c in s if c in "\n\t" or not unicodedata.category(c).startswith("C"))


_FR = {
    "le",
    "la",
    "les",
    "des",
    "une",
    "un",
    "et",
    "vous",
    "nous",
    "pour",
    "avec",
    "est",
    "dans",
    "sur",
    "notre",
    "votre",
    "être",
    "du",
    "au",
}
_EN = {
    "the",
    "and",
    "you",
    "we",
    "for",
    "with",
    "is",
    "in",
    "on",
    "our",
    "your",
    "to",
    "be",
    "of",
    "a",
    "an",
    "will",
}


def language(text: str) -> str:
    """ "fr" or "en", by counting common words: enough to stratify a sample."""
    words = [w.strip(".,;:!?()'\"").casefold() for w in text.split()]
    fr = sum(w in _FR for w in words)
    en = sum(w in _EN for w in words)
    return "fr" if fr > en else "en"


MAX_CHARS = 12_000  # ~3k tokens: longer than all but a few postings (M3-07 set max 13.9k)

# Section headings whose content never holds requirements (EN, FR, DE).
_BOILERPLATE = re.compile(
    r"^(about (us|the company|the team at)|about [a-z0-9 .&-]{1,30}$|who we are|our story"
    r"|benefits|perks|what we offer|what('s| is) in it for you|why (join|you should|work)"
    r"|(our )?(diversity|equal (employment )?opportunit|eeo|inclusion)|privacy"
    r"|accommodation|(your )?data privacy|legal|à propos|qui sommes|nos avantages"
    r"|pourquoi nous rejoindre|über uns|was wir (dir |ihnen )?bieten|benefits und"
    r"|the good stuff|life at |culture)",
    re.IGNORECASE,
)


def _is_heading(line: str) -> bool:
    s = line.strip()
    return 0 < len(s) <= 80 and not s.startswith("-") and not s.endswith((".", ",", ";"))


def bound(text: str, max_chars: int = MAX_CHARS) -> str:
    """Fits prepared posting text in max_chars (M3-04 AC6). Boilerplate sections (about
    the company, benefits, EEO and legal text) go first, last one first; only when none
    is left is the end cut, so requirements are never cut while boilerplate remains."""
    if len(text) <= max_chars:
        return text
    sections: list[list[str]] = [[]]
    for line in text.split("\n"):
        if _is_heading(line) and sections[-1]:
            sections.append([])
        sections[-1].append(line)
    boiler = [k for k, sec in enumerate(sections) if k and _BOILERPLATE.match(sec[0].strip())]
    dropped: set[int] = set()

    def joined() -> str:
        return "\n".join(line for k, sec in enumerate(sections) if k not in dropped for line in sec)

    out = joined()
    for k in reversed(boiler):
        if len(out) <= max_chars:
            break
        dropped.add(k)
        out = joined()
    return out[:max_chars]
