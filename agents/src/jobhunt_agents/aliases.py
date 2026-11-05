"""The committed tech-alias table (tech_aliases.yaml): canonical names for common other
spellings, so "GCP" and "Google Cloud Platform" are one skill. Not personal; the owner's
skill-map aliases apply on top (profile.py)."""

import unicodedata
from importlib import resources
from re import compile as _re
from typing import cast

from ruamel.yaml import YAML

_SEP = _re(r"[\s\-_.]+")


def normalize(s: str) -> str:
    """M3-05's folding: case, accents and separators don't matter
    ("Développeur Node.js" ~ "developpeur node-js")."""
    s = unicodedata.normalize("NFKD", s)
    s = "".join(c for c in s if not unicodedata.combining(c))
    return _SEP.sub(" ", s.casefold()).strip()


def _load() -> dict[str, tuple[str, ...]]:
    text = (resources.files(__package__) / "tech_aliases.yaml").read_text()
    raw = cast(dict[str, list[str]], YAML(typ="safe").load(text))  # pyright: ignore[reportUnknownMemberType]
    return {str(k): tuple(str(a) for a in v) for k, v in raw.items()}


TABLE = _load()
_CANON: dict[str, str] = {}
for _name, _aliases in TABLE.items():
    for _a in (_name, *_aliases):
        if _CANON.setdefault(normalize(_a), normalize(_name)) != normalize(_name):
            raise ValueError(f"tech_aliases.yaml: {_a!r} is under two names")


def canon(s: str) -> str:
    """The folded canonical name: the table's name when s is one of its spellings."""
    f = normalize(s)
    return _CANON.get(f, f)
