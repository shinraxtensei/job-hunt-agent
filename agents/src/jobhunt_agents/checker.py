"""The claim checker (M5-02): an independent model pass that judges every sentence of a
draft (CV and letter) against the evidence lines it cites (evidence-bank-guardrails).

It runs only on drafts the code validator (M4-04) passed, and its context holds the
sentences, their cited lines and the bank's exact facts: never the posting's text, its
skill names or the tailoring prompt, so a malicious posting can't steer it. A verdict
other than `supported` blocks the draft.
"""

from dataclasses import dataclass
from typing import Any, Literal

from pydantic import BaseModel, ConfigDict, Field

from jobhunt_agents.evidence import Bank
from jobhunt_agents.model import InvalidOutput, ModelClient, Prompt
from jobhunt_agents.validator import Finding

AGENT = "checker"
PROMPT_VERSION = 1
MAX_TOKENS = 12000  # thinking on: judging is reasoning


class Verdict(BaseModel):
    model_config = ConfigDict(extra="forbid")
    n: int  # the sentence's number in the data
    verdict: Literal["supported", "unsupported", "altered_fact"]
    reason: str = Field(max_length=300)
    closest: str | None  # the evidence id the sentence most resembles


class Verdicts(BaseModel):
    model_config = ConfigDict(extra="forbid")
    verdicts: list[Verdict]


@dataclass(frozen=True)
class Sentence:
    where: str  # as the validator names it: summary[0], roles[x].bullets[1], letter[0][2]
    text: str
    evidence_ids: tuple[str, ...]


def sentences(content: dict[str, Any], letter: dict[str, Any] | None) -> list[Sentence]:
    """Every model-written sentence of a rendered draft, in reading order."""
    out = [
        Sentence(f"summary[{i}]", b["text"], tuple(b["evidence_ids"]))
        for i, b in enumerate(content["summary"])
    ]
    for kind in ("roles", "projects"):
        for sec in content[kind]:
            out += [
                Sentence(f"{kind}[{sec['id']}].bullets[{i}]", b["text"], tuple(b["evidence_ids"]))
                for i, b in enumerate(sec["bullets"])
            ]
    for p, para in enumerate((letter or {}).get("paragraphs", [])):
        out += [
            Sentence(f"letter[{p}][{i}]", b["text"], tuple(b["evidence_ids"]))
            for i, b in enumerate(para)
        ]
    return out


def check_data(items: list[Sentence], bank: Bank) -> str:
    """What the checker reads: the bank's facts, then each sentence with its cited lines."""
    out = ["Facts (exact):"]
    out += [f"- role: {r.title} at {r.employer}, {r.start} to {r.end}" for r in bank.roles]
    out += [f"- project: {p.name}" for p in bank.projects]
    out += [f"- education: {e.degree}, {e.school}, {e.start} to {e.end}" for e in bank.education]
    out += [f"- certification: {c.name}, {c.issuer}, {c.date}" for c in bank.certifications]
    lines = {x.id: x.text for x in bank.lines()}
    for n, s in enumerate(items):
        out += ["", f"Sentence {n}: {s.text}"]
        out += [f"  cites [{i}] {lines.get(i, '(not in the bank)')}" for i in s.evidence_ids]
    return "\n".join(out)


class Checker:
    def __init__(self, client: ModelClient, prompt: Prompt | None = None) -> None:
        self.client = client
        self.prompt = prompt or Prompt.load(AGENT, PROMPT_VERSION, cacheable=False)

    def _ask(self, data: str, posting_id: int | None) -> Verdicts:
        return self.client.structured(
            agent=AGENT,
            prompt=self.prompt,
            data=data,
            output=Verdicts,
            max_tokens=MAX_TOKENS,
            posting_id=posting_id,
            thinking=True,  # measured: more consistent verdicts (M5-02 evals)
        )

    def check(
        self, items: list[Sentence], bank: Bank, posting_id: int | None = None
    ) -> list[Finding]:
        """One verdict per sentence, or InvalidOutput (after one retry): never a guess."""
        if not items:
            return []
        data = check_data(items, bank)
        known = {x.id for x in bank.lines()}
        for _ in range(2):
            got = self._ask(data, posting_id).verdicts
            if sorted(v.n for v in got) == list(range(len(items))):
                break
        else:
            raise InvalidOutput("the checker's verdicts don't match the sentences")
        return [
            Finding(
                "checker_unsupported" if v.verdict == "unsupported" else "checker_altered_fact",
                items[v.n].where,
                items[v.n].text,
                v.closest if v.closest in known else None,
                v.reason,
            )
            for v in sorted(got, key=lambda v: v.n)
            if v.verdict != "supported"
        ]
