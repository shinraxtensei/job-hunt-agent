"""The extraction agent (M3-04): one posting's facts as an `Extraction`.

The posting is untrusted: it is prepared as plain text (text.py), bounded, and sent only
inside the model client's escaped <data> block; the prompt says it is data.
"""

from typing import Any

from psycopg.types.json import Jsonb
from psycopg_pool import ConnectionPool

from jobhunt_agents.extraction import Extraction
from jobhunt_agents.model import MODEL, ModelClient, Prompt
from jobhunt_agents.queue import Handler, Item, ItemError
from jobhunt_agents.text import bound, posting_text

AGENT = "extract"
PROMPT_VERSION = 2  # a new prompt is a new file; bump this to ship it (AC5, AC7)
MAX_TOKENS = 2048  # outputs average ~900 tokens; billed on use


def prepare(source: str, payload: dict[str, Any], *, title: str, location: str) -> str:
    """What the model reads (AC6): title, location and description, within the bound."""
    return bound(posting_text(source, payload, title=title, location=location))


class Extractor:
    def __init__(self, client: ModelClient, prompt: Prompt | None = None) -> None:
        self.client = client
        self.prompt = prompt or Prompt.load(AGENT, PROMPT_VERSION)

    def __call__(self, text: str, posting_id: int | None = None) -> Extraction:
        """Raises the model client's NeedsReview errors when the output stays invalid."""
        return self.client.structured(
            agent=AGENT,
            prompt=self.prompt,
            data=text,
            output=Extraction,
            max_tokens=MAX_TOKENS,
            posting_id=posting_id,
        )


_POSTING = """
SELECT p.source, p.raw_payload, p.content_hash, j.title, j.locations
FROM postings p JOIN jobs j ON j.posting_id = p.id
WHERE p.id = %s
"""

_DONE = """
SELECT 1 FROM extractions WHERE posting_id = %s AND content_hash = %s AND prompt_version = %s
"""

_STORE = """
INSERT INTO extractions (posting_id, content_hash, prompt_version, model, data)
VALUES (%s, %s, %s, %s, %s)
ON CONFLICT (posting_id, content_hash, prompt_version) DO NOTHING
"""

_REGISTER = """
INSERT INTO agent_prompts (agent, version) VALUES (%s, %s)
ON CONFLICT (agent) DO UPDATE SET version = excluded.version, updated_at = now()
"""


def register_prompt(pool: ConnectionPool) -> None:
    """Records the prompt version this worker runs; Go's --reextract compares to it."""
    with pool.connection() as conn:
        conn.execute(_REGISTER, (AGENT, PROMPT_VERSION))


def handler(pool: ConnectionPool, extract: Extractor) -> Handler:
    """The `extract` work-item handler. Idempotent on the posting's content hash and the
    prompt version (AC4): content already extracted with this prompt is not sent again.
    NeedsReview from the model client fails the item at once (AC2); nothing is stored."""
    version = extract.prompt.version

    def run(item: Item) -> None:
        with pool.connection() as conn:
            row = conn.execute(_POSTING, (item.posting_id,)).fetchone()
            if row is None:
                raise ItemError("posting or job not found")
            source, payload, content_hash, title, locations = row
            if conn.execute(_DONE, (item.posting_id, content_hash, version)).fetchone():
                return
        text = prepare(source, payload, title=title, location=", ".join(locations) or "unknown")
        got = extract(text, item.posting_id)
        with pool.connection() as conn:
            conn.execute(
                _STORE,
                (item.posting_id, content_hash, version, MODEL, Jsonb(got.model_dump(mode="json"))),
            )

    return run
