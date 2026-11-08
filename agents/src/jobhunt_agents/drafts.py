"""The CV draft's content (M4-04 defines it, M4-02 produces it). The model writes only
these sentences and the skills list; the header and the exact facts (titles, employers,
dates, degrees) are filled in by code from the evidence bank, never by the model."""

from pydantic import BaseModel, ConfigDict, Field


class Bullet(BaseModel):
    model_config = ConfigDict(extra="forbid")
    text: str = Field(min_length=1, max_length=400)
    evidence_ids: list[str]  # the evidence lines this sentence is built from


class Section(BaseModel):
    model_config = ConfigDict(extra="forbid")
    id: str  # a role or project id from the evidence bank
    bullets: list[Bullet]


class DraftContent(BaseModel):
    model_config = ConfigDict(extra="forbid")
    summary: list[Bullet]  # 1 or 2 sentences
    roles: list[Section]
    projects: list[Section]
    skills: list[str]  # the skills section: proven and familiar skills only
