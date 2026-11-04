# M3-03: `decide()` interface and model client

- **Status:** Ready
- **Milestone:** M3 — Extraction and scoring
- **Linear:** [HAM-42](https://linear.app/hamid-corp/issue/HAM-42/m3-03-decide-interface-and-model-client)
- **Estimate:** 1 day

## Story

As the job seeker, I want every model call to go through one client that validates
output, logs its cost and stops at a daily budget, so that extraction and scoring are
debuggable and can never run up a bill.

## Context

ADR-0003 (call the provider SDK directly, no LangChain; one `decide()` interface for
yes/no and pick-one judgments) is still Proposed: this story **Accepts or revises** it.
The PRD target is < $20/month of model cost. The provider and model(s) are not chosen yet
(PRD open question).

## Acceptance criteria

1. **Given** the choice of provider and model(s)
   **When** this story starts
   **Then** an ADR records the provider, the model for each task (extraction, scoring
   reasons, `decide()`), and an **estimated cost per lead and per month** for ~500 shown
   leads (with the token assumptions). The estimate weighs the provider's **Batch API**
   (about 50% cheaper; its latency is fine for a morning tracker) and **prompt caching**
   of the fixed instructions, and the ADR decides whether M3 uses batches or sync calls,
   and why. Approved before the client is written.
2. **Given** a call with a Pydantic output model
   **When** the provider returns malformed or schema-invalid output
   **Then** it is retried once; a second failure raises a typed error the caller turns
   into `needs_review`. Never guessed or repaired by hand (tests with a fake provider).
3. **Given** any call
   **When** it completes or fails
   **Then** one `model_calls` row records: agent, prompt file and version, model, input
   and output tokens, cost, latency, outcome, the job/posting id. No prompt or response
   text is stored by default.
4. **Given** a daily budget cap in configuration (`MODEL_DAILY_BUDGET_USD`)
   **When** today's logged cost reaches it
   **Then** the client refuses new calls and the worker stops claiming work until the
   next day (UTC), logging why; a test pushes the fake provider past the cap.
5. **Given** the API key
   **When** the worker runs
   **Then** it comes only from `.env` (Pydantic `SecretStr`), is never logged, never in
   errors, and the image and repository contain no key (test asserts redaction).
6. **Given** `decide(question, options, context)`
   **When** called
   **Then** it returns one of the options (validated) and logs like any call; callers
   depend only on this interface (a test swaps the model behind it).

## Out of scope

- Prompts for extraction and scoring (M3-04, M3-06). Caching responses (prompt caching
  of fixed instructions is the provider's feature, decided in the ADR).

## Security notes

New network access: the provider's API only (fixed base URL from the SDK). Only the text
an agent needs is sent (PRD: personal data stays local). Untrusted posting text is always
passed as data in a delimited user message, never concatenated into instructions.

## Notes for the implementer

CI never calls the real provider: a fake provider implements the same protocol. The cost
table (price per token per model) is configuration with its source date.
