"""The fake model provider for tests: CI never calls the real API (M3-03)."""

from jobhunt_agents.model import Request, Response, Usage


def answer(
    text: str, *, stop: str = "end_turn", tokens: tuple[int, int] = (1000, 100), cached: int = 0
) -> Response:
    return Response(
        text=text,
        stop_reason=stop,
        usage=Usage(
            input_tokens=tokens[0], output_tokens=tokens[1], cache_read_input_tokens=cached
        ),
    )


class FakeProvider:
    """Returns (or raises) the scripted answers in order and records every request."""

    def __init__(self, *answers: Response | Exception) -> None:
        self.answers = list(answers)
        self.requests: list[Request] = []

    def call(self, request: Request) -> Response:
        self.requests.append(request)
        a = self.answers.pop(0)
        if isinstance(a, Exception):
            raise a
        return a
