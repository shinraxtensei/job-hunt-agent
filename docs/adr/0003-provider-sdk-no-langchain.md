# 0003. Call the model provider SDK directly, no LangChain

- **Status:** Proposed
- **Date:** 2026-10-31
- **Story:** Milestone 3 — extraction and scoring

## Context

The agents in v1 are narrow: extract structured facts, score, tailor, check claims. Each
is a single model call (or a call plus one retry) with a strict input and output schema.
The owner already knows LangChain from work and wants this project to teach how agents
work underneath. Debuggability matters: when a score is wrong, we need to see the exact
prompt, response, tokens, and cost.

## Decision

Use the model provider's official Python SDK directly. Every input and output is a
Pydantic model; structured output is requested from the provider and validated again on
our side. Prompts are versioned files in `agents/<agent>/prompts/`. Yes/no and pick-one
judgments go through a single `decide()` interface so the model behind it can change
without touching callers (PRD: decision layer).

## Alternatives considered

- **LangChain / LangGraph:** useful for multi-step tool-using agents; here it would add
  abstraction layers over single calls and hide the prompts we need to see.
- **A multi-provider wrapper library:** adds a dependency to solve a problem we don't have
  yet; the `decide()` interface already isolates the model choice.

## Consequences

- **Good:** full visibility of prompts and responses; fewer dependencies; easy to log
  cost per call; strong learning value.
- **Bad:** we write our own retry, validation, and logging helpers (small); switching
  providers means changing one client module.
- **Revisit when:** an agent genuinely needs multi-step tool use or long-running state.
