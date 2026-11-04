"""M3-01 AC4: configuration comes only from environment variables, validated once."""

import pytest

from jobhunt_agents.settings import ConfigError, Settings, load

SECRET = "postgres://jobhunt_app:s3cret-pw@db:5432/jobhunt"


def test_reads_environment(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("DATABASE_URL", SECRET)
    monkeypatch.setenv("WORKER_POLL_SECONDS", "2.5")
    s = load()
    assert isinstance(s, Settings)
    assert s.database_url.get_secret_value() == SECRET
    assert s.worker_poll_seconds == 2.5
    assert s.worker_pool_max == 2  # default


def test_secret_never_printed(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("DATABASE_URL", SECRET)
    s = load()
    assert "s3cret-pw" not in repr(s)
    assert "s3cret-pw" not in str(s)
    assert "s3cret-pw" not in s.model_dump_json()


def test_missing_variable_is_named(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("DATABASE_URL", raising=False)
    with pytest.raises(ConfigError, match="DATABASE_URL"):
        load()


@pytest.mark.parametrize(
    ("name", "value"),
    [
        ("WORKER_POLL_SECONDS", "not-a-number-xyz"),
        ("WORKER_POLL_SECONDS", "0"),
        ("WORKER_POOL_MAX", "999"),
        ("LOG_LEVEL", "LOUD-xyz"),
    ],
)
def test_invalid_value_names_variable_not_value(
    monkeypatch: pytest.MonkeyPatch, name: str, value: str
) -> None:
    monkeypatch.setenv("DATABASE_URL", SECRET)
    monkeypatch.setenv(name, value)
    with pytest.raises(ConfigError) as err:
        load()
    assert name in str(err.value)
    assert value not in str(err.value)
    assert "s3cret-pw" not in str(err.value)


def test_database_url_value_never_in_error(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("DATABASE_URL", SECRET)
    monkeypatch.setenv("WORKER_POOL_MAX", "0")
    with pytest.raises(ConfigError) as err:
        load()
    assert "s3cret-pw" not in str(err.value)


# M3-03 AC5: the API key is a secret: never in reprs or dumps; optional (no key, no
# model work); the budget cap defaults to $1.00 a day.
def test_api_key_and_budget(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("DATABASE_URL", SECRET)
    assert load().anthropic_api_key is None
    monkeypatch.setenv("ANTHROPIC_API_KEY", "")  # ANTHROPIC_API_KEY= in .env: unset
    assert load().anthropic_api_key is None
    assert load().model_daily_budget_usd == 1
    monkeypatch.setenv("ANTHROPIC_API_KEY", "sk-ant-secret-xyz")
    s = load()
    assert s.anthropic_api_key is not None
    assert s.anthropic_api_key.get_secret_value() == "sk-ant-secret-xyz"
    assert "sk-ant-secret-xyz" not in repr(s) + s.model_dump_json()
    monkeypatch.setenv("MODEL_DAILY_BUDGET_USD", "-1")
    with pytest.raises(ConfigError, match="MODEL_DAILY_BUDGET_USD"):
        load()
