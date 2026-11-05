"""M3-04 AC6: preparing posting text for the prompt within a length bound."""

from jobhunt_agents.text import bound

FILLER = "We are a great place to work with many lovely people. " * 40


def long_posting() -> str:
    return "\n".join(
        [
            "Title: Backend Engineer",
            "Location: Berlin",
            "",
            "About Us",
            FILLER,
            "What we offer",
            FILLER,
            "Equal opportunity",
            "Acme is an equal opportunity employer. " * 30,
            "What you'll do",
            "- Build payment services in Go",
            "Requirements",
            "- 5+ years with Go",
            "- PostgreSQL and Kafka",
        ]
    )


def test_bound_keeps_short_text_as_is() -> None:
    assert bound("Title: X\n\nshort", 100) == "Title: X\n\nshort"


def test_bound_drops_boilerplate_before_cutting_requirements() -> None:
    text = long_posting()
    got = bound(text, 600)
    assert len(got) <= 600
    for kept in (
        "Title: Backend Engineer",
        "Location: Berlin",
        "Build payment services in Go",
        "5+ years with Go",
        "PostgreSQL and Kafka",
    ):
        assert kept in got
    assert "lovely people" not in got
    assert "equal opportunity employer" not in got


def test_bound_drops_only_as_much_boilerplate_as_needed() -> None:
    text = long_posting()
    got = bound(text, len(text) - 100)
    assert "Requirements" in got
    assert "lovely people" in got  # one boilerplate section was enough


def test_bound_cuts_the_end_when_nothing_is_boilerplate() -> None:
    text = "Title: X\n\nRequirements\n" + "- Go\n" * 200
    got = bound(text, 300)
    assert len(got) <= 300
    assert got.startswith("Title: X")
