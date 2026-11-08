"""The CV variant of a lead, chosen in code (M4-06; owner's direction 2026-11-08).

`decide()` chose DIST for 173 of 509 leads. The rule below reads what the tracker already
knows: the job's role family, its title, the extracted skills and the company's track.
It's free and deterministic, and its order says what wins: payments, then AI, then
client-facing work, then full-stack, then distributed systems; backend otherwise.
"""

import re

from jobhunt_agents.aliases import canon

VARIANTS = ("BE", "PAY", "DIST", "FS", "AI", "CONS")

_PAY_TITLE = re.compile(
    r"\b(payments?|fintech|billing|checkout|money|banking|card|acquiring)\b", re.I
)
_PAY_SKILLS = {
    canon(s)
    for s in (
        "Payments",
        "Payment integration",
        "Payment processing",
        "Fintech",
        "Billing",
        "KYC",
        "AML",
        "Fraud detection",
        "Card payments",
        "Acquiring",
        "PCI DSS",
    )
}
_AI_TITLE = re.compile(
    r"\b(ai|ml|llm|llms|machine learning|genai|data scientist|applied scientist|"
    r"research scientist|research engineer)\b",
    re.I,
)
_CONS_TITLE = re.compile(
    r"\b(customer engineer|solutions? (engineer|architect)|consultant|sales engineer|"
    r"forward deployed|professional services|technical account)\b",
    re.I,
)
_FS_TITLE = re.compile(r"\bfull[\s-]?stack\b", re.I)
_DIST_TITLE = re.compile(
    r"\b(distributed|infrastructure|site reliability|sre|devops|reliability|"
    r"data engineer|data infrastructure|storage|streaming)\b",
    re.I,
)
_CONS_FAMILIES = {"customer", "sales", "operations", "product"}
_FS_FAMILIES = {"fullstack", "frontend", "mobile"}
_DIST_FAMILIES = {"platform", "data"}


def cv_variant(role_family: str, title: str, skills: list[str], track: str) -> str:
    """BE, PAY, DIST, FS, AI or CONS, from the lead's facts."""
    if _PAY_TITLE.search(title) or {canon(s) for s in skills} & _PAY_SKILLS:
        return "PAY"
    if role_family == "ai_ml" or _AI_TITLE.search(title):
        return "AI"
    if role_family in _CONS_FAMILIES or track == "consulting" or _CONS_TITLE.search(title):
        return "CONS"
    if role_family in _FS_FAMILIES or _FS_TITLE.search(title):
        return "FS"
    if role_family in _DIST_FAMILIES or _DIST_TITLE.search(title):
        return "DIST"
    return "BE"


_PROXY = {
    "backend": "BE",
    "fullstack": "FS",
    "frontend": "FS",
    "mobile": "FS",
    "platform": "DIST",
    "data": "DIST",
    "ai_ml": "AI",
}


def proxy_variant(role_family: str) -> str:
    """The owner's fit-label role family as a stand-in label until OI-3 (role family
    only: it can't see payments or client-facing work)."""
    if role_family in _CONS_FAMILIES:
        return "CONS"
    return _PROXY.get(role_family, "BE")
