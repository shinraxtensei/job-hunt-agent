"""The model client (M3-03, ADR-0003, ADR-0018): every model call goes through it.

It validates output against a Pydantic model, retries once on schema-invalid or
truncated output, logs one `model_calls` row per call (never prompt or response text),
and refuses calls once today's (UTC) cost reaches the daily budget. The provider behind
it is a small protocol: Anthropic in production, a fake in tests.
"""

import html
import json
import logging
import time
from collections.abc import Callable
from dataclasses import dataclass
from decimal import Decimal
from importlib import resources
from typing import Any, Protocol

import anthropic
from psycopg_pool import ConnectionPool
from pydantic import BaseModel, ValidationError

from jobhunt_agents.queue import NeedsReview

log = logging.getLogger(__name__)

# Pinned exactly (ADR-0018).
MODEL = "claude-haiku-5-5"
ANTHROPIC_BASE_URL = "https://api.anthropic.com"

# USD per million tokens: input, output, cache write (5 min), cache read.
# Checked 2026-11-04 on platform.claude.com/docs/en/about-claude/pricing (ADR-0018).
# Haiku 5.5's prices for prompts under 100k tokens; ours stay far below (M3-04 AC6).
PRICES_CHECKED = "2026-11-04"
PRICES: dict[str, tuple[Decimal, Decimal, Decimal, Decimal]] = {
    "claude-haiku-5-5": (Decimal("0.10"), Decimal("0.50"), Decimal("0.125"), Decimal("0.01")),
    "claude-sonnet-5-5": (Decimal("2.00"), Decimal("10.00"), Decimal("2.50"), Decimal("0.10")),
}


def cost_usd(
    model: str, *, input_tokens: int, output_tokens: int, cache_read: int = 0, cache_write: int = 0
) -> Decimal:
    i, o, w, r = PRICES[model]
    total = input_tokens * i + output_tokens * o + cache_write * w + cache_read * r
    return (total / 1_000_000).quantize(Decimal("0.000001"))


@dataclass(frozen=True)
class Usage:
    input_tokens: int = 0  # uncached input only
    output_tokens: int = 0
    cache_read_input_tokens: int = 0
    cache_creation_input_tokens: int = 0


@dataclass(frozen=True)
class Request:
    model: str
    system: str  # the fixed instructions: the cached prefix
    user: str  # the variable, delimited data
    schema: dict[str, Any]
    max_tokens: int
    thinking: bool = True  # False for short calls: thinking can use the whole budget


@dataclass(frozen=True)
class Response:
    text: str
    stop_reason: str
    usage: Usage


class Provider(Protocol):
    def call(self, request: Request) -> Response: ...


class AnthropicProvider:
    """The Anthropic API: fixed base URL, structured output by JSON schema, the system
    prompt marked as the cache prefix."""

    def __init__(
        self,
        api_key: str,
        *,
        base_url: str = ANTHROPIC_BASE_URL,
        max_retries: int = 2,
        timeout: float = 60,
    ) -> None:
        self._client = anthropic.Anthropic(
            api_key=api_key, base_url=base_url, max_retries=max_retries, timeout=timeout
        )

    def __repr__(self) -> str:
        return "AnthropicProvider()"  # never the key

    def count_tokens(self, request: Request) -> int:
        """Input tokens the request would use (the API's free counting endpoint)."""
        r = self._client.messages.count_tokens(
            model=request.model,
            system=[{"type": "text", "text": request.system}],
            messages=[{"role": "user", "content": request.user}],
            output_config={"format": {"type": "json_schema", "schema": request.schema}},
        )
        return r.input_tokens

    def call(self, request: Request) -> Response:
        m = self._client.messages.create(
            model=request.model,
            max_tokens=request.max_tokens,
            system=[
                {"type": "text", "text": request.system, "cache_control": {"type": "ephemeral"}}
            ],
            messages=[{"role": "user", "content": request.user}],
            output_config={"format": {"type": "json_schema", "schema": request.schema}},
            thinking=anthropic.omit if request.thinking else {"type": "disabled"},
        )
        u = m.usage
        return Response(
            text="".join(b.text for b in m.content if b.type == "text"),
            stop_reason=m.stop_reason or "",
            usage=Usage(
                input_tokens=u.input_tokens,
                output_tokens=u.output_tokens,
                cache_read_input_tokens=u.cache_read_input_tokens or 0,
                cache_creation_input_tokens=u.cache_creation_input_tokens or 0,
            ),
        )


@dataclass(frozen=True)
class Prompt:
    """A versioned prompt file: jobhunt_agents/prompts/<agent>/v<N>.md."""

    agent: str
    file: str
    version: int
    text: str
    # False for prompts shorter than the 512-token cache minimum by design (ADR-0018):
    # their calls are never cached, and that is expected, not worth a warning.
    cacheable: bool = True

    @classmethod
    def load(cls, agent: str, version: int, *, cacheable: bool = True) -> Prompt:
        file = f"{agent}/v{version}.md"
        text = (resources.files("jobhunt_agents") / "prompts" / file).read_text()
        return cls(agent=agent, file=file, version=version, text=text, cacheable=cacheable)


# Errors the caller turns into needs_review (the item fails at once). Their messages
# never hold model output.
class InvalidOutput(NeedsReview):
    pass


class ModelRefused(NeedsReview):
    pass


class Truncated(NeedsReview):
    pass


class BudgetExceeded(Exception):
    """Today's (UTC) model cost reached MODEL_DAILY_BUDGET_USD."""


def delimit(data: str) -> str:
    """Untrusted text as data in a block it can't close early: every &, < and > in it is
    HTML-escaped, so no spelling of a closing tag survives (the prompts say so)."""
    return f"<data>\n{html.escape(data, quote=False)}\n</data>"


def _rule(e: ValidationError | ValueError) -> str:
    """The rule an output broke, safe to store: the field and error type, or the fixed
    message of one of our validators. Never the output (pydantic's input_value)."""
    if not isinstance(e, ValidationError):
        return str(e)  # our own parse functions' fixed messages
    err = e.errors(include_url=False, include_input=False)[0]
    if err["type"] == "value_error":
        return str(err.get("ctx", {}).get("error", "value_error"))[:120]
    loc = ".".join(str(x) for x in err["loc"])
    return f"{loc}: {err['type']}" if loc else err["type"]


class Decide(Protocol):
    """What callers depend on for yes/no and pick-one judgments (ADR-0003)."""

    def __call__(
        self, question: str, options: list[str], context: str, *, posting_id: int | None = None
    ) -> str: ...


class _Choice(BaseModel):
    choice: str


_SPENT_TODAY = """
SELECT coalesce(sum(cost_usd), 0) FROM model_calls
WHERE created_at >= date_trunc('day', now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'
"""

_LOG_CALL = """
INSERT INTO model_calls (agent, prompt_file, prompt_version, model, input_tokens,
    output_tokens, cache_read_input_tokens, cache_creation_input_tokens, cost_usd,
    latency_ms, outcome, posting_id)
VALUES (%(agent)s, %(file)s, %(version)s, %(model)s, %(input)s, %(output)s, %(cache_read)s,
    %(cache_write)s, %(cost)s, %(latency)s, %(outcome)s, %(posting_id)s)
"""


class ModelClient:
    def __init__(
        self,
        *,
        provider: Provider,
        pool: ConnectionPool,
        daily_budget_usd: Decimal,
        model: str = MODEL,
    ) -> None:
        self._provider = provider
        self._pool = pool
        self._budget = daily_budget_usd
        self._model = model
        self._warned_uncached: set[str] = set()  # agents, once per process

    def spent_today(self) -> Decimal:
        with self._pool.connection() as conn:
            row = conn.execute(_SPENT_TODAY).fetchone()
        return Decimal(row[0]) if row else Decimal(0)

    def budget_exhausted(self) -> bool:
        return self.spent_today() >= self._budget

    def structured[T: BaseModel](
        self,
        *,
        agent: str,
        prompt: Prompt,
        data: str,
        output: type[T],
        max_tokens: int,
        posting_id: int | None = None,
        thinking: bool = True,
    ) -> T:
        """One structured call: prompt.text is the fixed instructions, data is untrusted."""
        return self._call(
            thinking=thinking,
            agent=agent,
            prompt=prompt,
            user=delimit(data),
            schema=anthropic.transform_schema(output),
            parse=output.model_validate_json,
            max_tokens=max_tokens,
            posting_id=posting_id,
        )

    def decide(
        self, question: str, options: list[str], context: str, *, posting_id: int | None = None
    ) -> str:
        """Returns one of options. question and options come from our code; context is
        untrusted data."""
        folded = [o.casefold() for o in options]
        if len(options) < 2 or len(set(folded)) != len(options):
            raise ValueError("decide needs at least two options, distinct ignoring case")

        def parse(text: str) -> str:
            got = _Choice.model_validate_json(text).choice.casefold()
            if got not in folded:
                raise ValueError("choice is not one of the options")
            return options[folded.index(got)]

        user = f"Question: {question}\nOptions: {json.dumps(options)}\n{delimit(context)}"
        schema = {
            "type": "object",
            "properties": {"choice": {"type": "string", "enum": list(options)}},
            "required": ["choice"],
            "additionalProperties": False,
        }
        return self._call(
            agent="decide",
            prompt=Prompt.load("decide", 1, cacheable=False),  # short by design
            user=user,
            schema=schema,
            parse=parse,
            max_tokens=50,
            posting_id=posting_id,
            thinking=False,  # one word; thinking could use all 50 tokens
        )

    def _call[T](
        self,
        *,
        agent: str,
        prompt: Prompt,
        user: str,
        schema: dict[str, Any],
        parse: Callable[[str], T],
        max_tokens: int,
        posting_id: int | None,
        thinking: bool = True,
    ) -> T:
        # One retry for invalid or truncated output (truncated: with twice the room).
        for attempt in (1, 2):
            if self.budget_exhausted():
                raise BudgetExceeded("daily model budget reached")
            req = Request(self._model, prompt.text, user, schema, max_tokens, thinking)
            began = time.monotonic()
            try:
                resp = self._provider.call(req)
            except Exception:
                self._log(
                    agent,
                    prompt=prompt,
                    u=Usage(),
                    began=began,
                    outcome="error",
                    posting_id=posting_id,
                )
                raise
            if resp.stop_reason == "refusal":
                self._log(
                    agent,
                    prompt=prompt,
                    u=resp.usage,
                    began=began,
                    outcome="refusal",
                    posting_id=posting_id,
                )
                raise ModelRefused("the model refused")
            if resp.stop_reason == "max_tokens":
                self._log(
                    agent,
                    prompt=prompt,
                    u=resp.usage,
                    began=began,
                    outcome="max_tokens",
                    posting_id=posting_id,
                )
                if attempt == 2:
                    raise Truncated("output truncated twice")
                max_tokens *= 2
                continue
            try:
                value = parse(resp.text)
            except (ValidationError, ValueError) as e:
                rule = _rule(e)
                self._log(
                    agent,
                    prompt=prompt,
                    u=resp.usage,
                    began=began,
                    outcome="invalid",
                    posting_id=posting_id,
                )
                if attempt == 2:
                    raise InvalidOutput(f"schema-invalid output twice ({rule})") from None
                continue
            self._log(
                agent, prompt=prompt, u=resp.usage, began=began, outcome="ok", posting_id=posting_id
            )
            uncached = not (
                resp.usage.cache_read_input_tokens or resp.usage.cache_creation_input_tokens
            )
            if uncached and prompt.cacheable and agent not in self._warned_uncached:
                self._warned_uncached.add(agent)
                log.warning("prompt prefix not cached (under 512 tokens?)", extra={"agent": agent})
            return value
        raise AssertionError("unreachable")  # pragma: no cover

    def _log(
        self,
        agent: str,
        *,
        prompt: Prompt,
        u: Usage,
        began: float,
        outcome: str,
        posting_id: int | None,
    ) -> None:
        cost = cost_usd(
            self._model,
            input_tokens=u.input_tokens,
            output_tokens=u.output_tokens,
            cache_read=u.cache_read_input_tokens,
            cache_write=u.cache_creation_input_tokens,
        )
        fields = {
            "agent": agent,
            "file": prompt.file,
            "version": prompt.version,
            "model": self._model,
            "input": u.input_tokens,
            "output": u.output_tokens,
            "cache_read": u.cache_read_input_tokens,
            "cache_write": u.cache_creation_input_tokens,
            "cost": cost,
            "latency": round((time.monotonic() - began) * 1000),
            "outcome": outcome,
            "posting_id": posting_id,
        }
        with self._pool.connection() as conn:
            conn.execute(_LOG_CALL, fields)
        log.info("model call", extra={k: fields[k] for k in ("agent", "outcome", "cost")})
