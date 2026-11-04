"""Configuration from environment variables only, validated once at start-up (M3-01 AC4)."""

from decimal import Decimal
from pathlib import Path
from typing import Literal

from pydantic import Field, SecretStr, ValidationError
from pydantic_settings import BaseSettings, SettingsConfigDict


class ConfigError(Exception):
    """A missing or invalid variable. The message names it and never holds its value."""


class Settings(BaseSettings):
    # No env_file: Compose passes the environment; .env is never read from here.
    # env_ignore_empty: an empty variable (ANTHROPIC_API_KEY= in .env) means unset.
    model_config = SettingsConfigDict(
        case_sensitive=False, extra="ignore", frozen=True, env_ignore_empty=True
    )

    database_url: SecretStr
    worker_poll_seconds: float = Field(default=5.0, gt=0, le=300)
    worker_pool_max: int = Field(default=2, ge=1, le=10)
    # A claimed item is someone else's after this long without an acknowledgement.
    worker_lease_seconds: float = Field(default=600, ge=10, le=3600)
    # The database unreachable this long is fatal (Compose restarts the worker).
    worker_db_outage_seconds: float = Field(default=60, gt=0, le=3600)
    log_level: Literal["DEBUG", "INFO", "WARNING", "ERROR"] = "INFO"
    # M3-03, ADR-0018: no key, no model work (handlers that need the model aren't run).
    anthropic_api_key: SecretStr | None = None
    # Today's (UTC) model cost at which the worker stops claiming work.
    model_daily_budget_usd: Decimal = Field(default=Decimal("1.00"), ge=0, le=100)
    heartbeat_file: Path = Path("/tmp/jobhunt-worker.heartbeat")  # noqa: S108 - a tmpfs in Compose


def load() -> Settings:
    try:
        return Settings()  # pyright: ignore[reportCallIssue] - fields come from the environment
    except ValidationError as e:
        # Only the location and the kind of error: include_input=False keeps values out.
        problems = sorted(
            {
                f"{'.'.join(str(p) for p in err['loc']).upper()} ({err['type']})"
                for err in e.errors(include_input=False, include_url=False, include_context=False)
            }
        )
        raise ConfigError("invalid configuration: " + ", ".join(problems)) from None
