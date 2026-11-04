"""M3-03: the real provider adapter against a local stand-in for the API (CI never calls
the real one): the request it sends, the response it reads, and that the key never
leaks."""

import json
import threading
from collections.abc import Iterator
from http.server import BaseHTTPRequestHandler, HTTPServer

import anthropic
import pytest

from jobhunt_agents.model import AnthropicProvider, Request

KEY = "sk-ant-test-key-do-not-print"

OK = {
    "id": "msg_1",
    "type": "message",
    "role": "assistant",
    "model": "claude-haiku-5-5",
    "content": [{"type": "text", "text": '{"choice": "yes"}'}],
    "stop_reason": "end_turn",
    "stop_sequence": None,
    "usage": {
        "input_tokens": 40,
        "output_tokens": 7,
        "cache_read_input_tokens": 600,
        "cache_creation_input_tokens": 0,
    },
}


class Server:
    def __init__(self) -> None:
        self.bodies: list[dict[str, object]] = []
        self.headers: list[dict[str, str]] = []
        self.status = 200
        server = self

        class Handler(BaseHTTPRequestHandler):
            def do_POST(self) -> None:
                n = int(self.headers["content-length"])
                server.bodies.append(json.loads(self.rfile.read(n)))
                server.headers.append({k.lower(): v for k, v in self.headers.items()})
                body = (
                    OK
                    if server.status == 200
                    else {
                        "type": "error",
                        "error": {"type": "authentication_error", "message": "invalid x-api-key"},
                    }
                )
                data = json.dumps(body).encode()
                self.send_response(server.status)
                self.send_header("content-type", "application/json")
                self.send_header("content-length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

            def log_message(self, format: str, *args: object) -> None:
                pass

        self.httpd = HTTPServer(("127.0.0.1", 0), Handler)
        self.url = f"http://127.0.0.1:{self.httpd.server_port}"
        threading.Thread(target=self.httpd.serve_forever, daemon=True).start()


@pytest.fixture
def api() -> Iterator[Server]:
    s = Server()
    yield s
    s.httpd.shutdown()


REQ = Request(
    model="claude-haiku-5-5",
    system="Fixed instructions.",
    user="<data>posting</data>",
    schema={"type": "object", "properties": {}, "additionalProperties": False},
    max_tokens=64,
)


def test_request_and_response(api: Server) -> None:
    resp = AnthropicProvider(KEY, base_url=api.url, max_retries=0).call(REQ)
    assert resp.text == '{"choice": "yes"}'
    assert resp.stop_reason == "end_turn"
    assert (
        resp.usage.input_tokens,
        resp.usage.output_tokens,
        resp.usage.cache_read_input_tokens,
    ) == (40, 7, 600)
    [body] = api.bodies
    assert body["model"] == "claude-haiku-5-5"
    assert body["max_tokens"] == 64
    # the fixed instructions are the cached prefix
    assert body["system"] == [
        {"type": "text", "text": "Fixed instructions.", "cache_control": {"type": "ephemeral"}}
    ]
    assert body["messages"] == [{"role": "user", "content": "<data>posting</data>"}]
    assert body["output_config"] == {"format": {"type": "json_schema", "schema": REQ.schema}}
    assert api.headers[0]["authorization"] == f"Bearer {KEY}"  # SDK 2.x sends the key this way


# AC5: the key never appears in reprs or errors.
def test_key_never_leaks(api: Server) -> None:
    api.status = 401
    p = AnthropicProvider(KEY, base_url=api.url, max_retries=0)
    assert KEY not in repr(p)
    with pytest.raises(anthropic.AuthenticationError) as err:
        p.call(REQ)
    assert KEY not in str(err.value)
    assert KEY not in repr(err.value)
