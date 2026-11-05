"""The facts extraction produces for one posting (M3-04 AC1). Defined here first so the
eval labels (M3-07) have a fixed shape before the extraction prompt exists."""

from typing import Literal

from pydantic import BaseModel, ConfigDict, Field

Seniority = Literal["junior", "mid", "senior", "lead", "unknown"]
YesNo = Literal["yes", "no", "unknown"]
RemotePolicy = Literal["remote", "hybrid", "onsite", "unknown"]
Period = Literal["year", "month", "day", "hour"]


class Salary(BaseModel):
    model_config = ConfigDict(frozen=True, extra="forbid")
    min: int = Field(ge=0)
    max: int = Field(ge=0)
    currency: str = Field(pattern=r"^[A-Z]{3}$")
    period: Period


class Extraction(BaseModel):
    """`unknown`, empty lists and null are valid answers: nothing is invented."""

    model_config = ConfigDict(frozen=True, extra="forbid")
    required_skills: list[str]
    optional_skills: list[str]
    min_years: int | None = Field(ge=0, le=50)
    seniority: Seniority
    visa_sponsorship: YesNo
    salary: Salary | None
    languages: list[str]
    remote_policy: RemotePolicy


FIELDS: tuple[str, ...] = tuple(Extraction.model_fields)
LIST_FIELDS = ("required_skills", "optional_skills", "languages")
