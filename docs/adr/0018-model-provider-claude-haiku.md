# 0018. Use Claude Haiku 5.5 through the Anthropic API, sync calls with prompt caching

- **Status:** Accepted
- **Date:** 2026-11-04
- **Story:** docs/stories/M3-03-decide-and-model-client.md

## Context

M3 makes three kinds of model call, all through one client (ADR-0003):

- **Extraction (M3-04):** a posting's cleaned text in, a strict `Extraction` object out
  (structured output).
- **Scoring reasons and gaps (M3-06):** the extraction plus the needed skill names in, a
  short reason and a list of gaps out.
- **`decide()`:** a question, a few options and a short context in; one of the options
  out.

Calls are made only for leads the `default` preset shows (ADR-0001's reconcile step).
That is about 500 today, after a fresh database or a preset change. Once those are
done, only new or changed shown postings: tens a day. The PRD target is **< $20/month**.

### Prices

Price table **dated 2026-11-04** (the date each official page was checked). All in USD
per million tokens:

| Model | Input | Output | Cache write (5 min) | Cache read | Batch |
|---|---|---|---|---|---|
| Claude Haiku 5.5 (prompts ≤ 100k tokens) | 0.10 | 0.50 | 0.125 | 0.01 | −50% |
| Claude Sonnet 5.5 | 2.00 | 10.00 | 2.50 | 0.10 | −50% |
| OpenAI gpt-6-luna | 0.10 | 0.50 | no premium | 0.01 | −50% |
| Google gemini-3.1-flash-lite (paid tier) | 0.25 | 1.50 | 0.025 + $1/MTok/h storage | 0.025 | −50% |

- **Anthropic:** platform.claude.com/docs/en/about-claude/pricing and claude.com/pricing.
  Cache read is 0.1× input on Haiku 5.5 and 0.05× on Sonnet 5.5.
  - Caching stacks with the Batch discount.
  - The minimum cacheable prefix is 512 tokens.
  - Haiku 5.5 charges more only above 100k-token prompts; our prompts are under 4k.
- **OpenAI:** developers.openai.com/api/docs/pricing.
- **Google:** aistudio.google.com/docs/pricing. Gemini 3.8 Flash costs $0.75 in / $3.75
  out until 2026-12-31, then double.

### Token counts (an assumption until the first real run)

**These counts are an estimate, not Anthropic's numbers.** They come from OpenAI's
o200k tokenizer, plus 30% to allow for the Claude tokenizer counting more. On the first
real run (the M3-07 baseline), recount the same 20 postings with Anthropic's
token-counting endpoint (`messages.count_tokens`). Then update this table and the
estimate below from those counts and from the `usage` logged in `model_calls`.

I sampled 20 random shown leads from the local database (seed 20261104): 18 from
Greenhouse and 2 from Ashby. Each was cleaned to text (HTML parsed, `script` and
`style` dropped, lines kept), with no boilerplate removed yet, so these are upper
bounds. Counted with the o200k tokenizer:

- **Min:** 413 tokens
- **Median:** 1,156
- **Mean:** 1,098
- **p90:** 1,913
- **Max:** 1,936

The estimate below uses **1,430 tokens**: the mean × 1.3, to allow for the Claude
tokenizer counting more than o200k.

### Estimate per lead

Token assumptions:

| Call | Fixed instructions + schema (cacheable) | Variable input | Output |
|---|---|---|---|
| extract | 900 | 1,430 (the posting) | 300 |
| reasons | 700 | 450 (extraction summary + needed skill names) | 150 |
| `decide()` × 3 | 3 × 600 | 3 × 80 | 3 × 10 |

Every figure includes +5% for retries. "Worst case" means 500 leads every day; "steady
state" means 60 new or changed leads a day:

| Model | Mode | $/lead | $/month, worst case | $/month, steady state |
|---|---|---|---|---|
| Haiku 5.5 | sync, no cache | 0.00083 | 12.47 | 1.50 |
| **Haiku 5.5** | **sync + caching** | **0.00051** | **7.65** | **0.92** |
| Haiku 5.5 | batch + caching | 0.00025 | 3.82 | 0.46 |
| Sonnet 5.5 | sync + caching | 0.00984 | 147.64 | 17.72 |
| Sonnet 5.5 | batch + caching | 0.00492 | 73.82 | 8.86 |
| gpt-6-luna | sync + caching | 0.00051 | 7.65 | 0.92 |
| gemini-3.1-flash-lite | sync + caching | 0.00140 | 21.01 | 2.52 |

## Decision

- **Provider: Anthropic. Model id pinned exactly: `claude-haiku-5-5`, for all three
  tasks:** extraction, reasons and gaps, and `decide()`.
  - We call the official `anthropic` Python SDK (ADR-0003). It is pure Python; its
    compiled dependency `jiter` ships cp315 wheels.
  - The model id and the price table are configuration, with the date the prices were
    checked.
  - **Sonnet 5.5** is not used in M3. It is the comparison model for `make evals`
    (M3-07), and we escalate a task to it only if Haiku misses that task's eval
    threshold, through a new ADR. Sync with caching it would cost $148/month in the
    worst case.
- **Sync calls, not the Batch API, for M3.** Even the worst case ($7.65/month) is well
  under the target, and steady state is about $1/month. Batching would save at most
  ~$4/month, and:
  - Batch results take up to 24 h, which doesn't fit ADR-0001's lease model (a claimed
    item would outlive its lease).
  - It needs a submit/poll/collect state machine.
  - Anthropic's docs say a cache entry written during batch processing is likely to
    expire before it is read, so the two discounts don't reliably stack.
  - Revisit for bulk re-extraction (M3-04 `collector enqueue --reextract`) if one run
    would cost more than about $5.
- **Prompt caching on.** On Haiku 5.5, a prefix is cached only if it is **at least 512
  tokens**; a shorter one is sent uncached, with no error.
  - **The cached prefix:** each agent's system prompt is its fixed instructions plus its
    output schema written out, and it is marked with `cache_control` (5-minute TTL).
    Prompts for extraction and scoring are written to clear 512 tokens.
  - **The variable part:** the posting text, untrusted, comes after the prefix in a
    delimited user message.
  - **Logged per call:** `model_calls` records `cache_read_input_tokens` and
    `cache_creation_input_tokens`. A successful call that reports neither logs a
    warning, because its prefix was not cached.
  - **Where the prefix is under 512 tokens** (likely `decide()`, whose prompt is short),
    the "no caching" figures in the table are the real cost for that call.
  - The worker processes items back to back, so the cache stays warm through a run. The
    write premium is 1.25× once per 5 minutes. Expected saving: ~40% when the prefix
    qualifies.
- **Structured output is enforced by the API,** not by asking nicely:
  - The request sets `output_config.format` to the JSON schema built from the task's
    Pydantic model. The API constrains decoding to that schema.
  - We validate again with Pydantic (`messages.parse`). Schema features the API can't
    enforce, such as `minimum` and `maxLength`, are checked only on our side.
  - Enum values are lowercase and compared case-insensitively, because the docs warn the
    model may change their case.
- **Refusal and truncation.** We check the response's `stop_reason` before trusting the
  parsed result:
  - `refusal`: the output may not match the schema. Not retried, since the same input
    would likely be refused again. The item is marked `needs_review` with outcome
    `refusal`. Its tokens are billed and logged.
  - `max_tokens`: the output is truncated. Retried once with twice the `max_tokens`;
    if it is still cut, `needs_review`. `max_tokens` is set per agent at about 3× the
    expected output.
  - Schema-invalid output (anything `parse` rejects): retried once, then the typed
    error the caller turns into `needs_review` (M3-03 AC2). Never repaired by hand.
- **What leaves the Mac:** only what each call needs.
  - **Extraction:** the posting's title, location and cleaned description. These are
    public text, but untrusted.
  - **Reasons and gaps:** the extraction's fields and the **names** of the owner's
    matching or missing skills, e.g. "Go", "Kubernetes".
  - **Never sent:** CV text, evidence-bank entries, the owner's name, contact details,
    salary expectations or application history.
  - **`decide()`:** the question, the options and the short context its caller passes;
    callers follow the same rule.
- **Provider data terms**, checked 2026-11-04 (privacy.claude.com): API inputs and
  outputs are not used for training, and are normally deleted within 30 days. Content
  flagged for safety may be kept up to 2 years. Zero retention is available only by
  agreement with Anthropic. (Batch results, if ever used, are kept 29 days.)
- **Price tier:** the input length bound (M3-04 AC6) keeps every call far below
  Haiku 5.5's 100k-token price tier (our prompts are under 4k tokens).
- **The API key** comes only from `.env` as `ANTHROPIC_API_KEY` (a Pydantic
  `SecretStr`). It is never logged and never in errors. The SDK's fixed base URL is the
  only new network destination.
- **`model_calls` table** (a migration; written by the Python worker, read by both
  sides). One row per call:
  - agent, prompt file and version, model;
  - input, output, cache-read and cache-creation tokens;
  - cost, latency, outcome;
  - the posting id.
  - Never prompt or response text.
- **Daily budget cap:** `MODEL_DAILY_BUDGET_USD`, default **1.00**. The worst-case day
  costs about $0.26. When today's (UTC) logged cost reaches the cap, the client refuses
  new calls and the worker stops claiming until the next UTC day (M3-03 AC4).

## Alternatives considered

- **OpenAI gpt-6-luna:** the same price as Haiku 5.5 on paper, and strict JSON-schema
  outputs. It is not chosen because it would mean a second provider for no saving. The
  later agents (tailoring, claim checking) are expected to use Claude: one SDK, one key,
  one set of data terms, and one place prices change. Its retention terms are
  equivalent (30 days, no training by default).
- **Google gemini-3.1-flash-lite:** 2.5–3× Haiku 5.5 per lead at our input/output mix;
  explicit caching adds an hourly storage fee. The free tier uses content to improve
  products, so it would have to be the paid tier anyway.
- **Claude Sonnet 5.5 for everything:** better judgment, but 20× the price. Over the
  target in the worst case even with batches. Kept for evals.
- **Batch API now:** see the Decision. Cheaper, but it doesn't fit the queue's lease
  model, and it saves only a few dollars a month at this volume.
- **A local model (e.g. via Ollama):** no data leaves the Mac and it costs nothing per
  call. But extraction quality on messy postings, structured-output reliability, and
  running it on the server (milestone D) are all unproven. Revisit if the PRD's
  personal-data rule ever needs CV text sent to a model.

## Consequences

- **Good:** the cost is far under the target, so the cap is a safety net rather than a
  constraint. Schema-constrained output means few retries. One provider and one key.
- **Bad:**
  - The estimate rests on o200k token counts × 1.3 until M3-04 logs real usage.
  - Caching only pays when items are processed close together; one item a day pays
    full price, which is still under $0.001.
  - Haiku may miss the eval thresholds on hard postings (then Sonnet for that task,
    through an ADR).
  - Prices can change: the table is dated configuration, and `model_calls` shows the
    real spend.
- **Revisit when:**
  - real cost from `model_calls` passes $10/month;
  - a bulk re-extraction would cost more than $5 (use batches for it);
  - Haiku misses an eval threshold;
  - the provider changes its prices or data terms.
