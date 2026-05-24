# 0004 — Categorizer as a swappable interface

**Status:** Accepted

## Context

The categorization logic needs to call an AI model. The choice of model (Anthropic API, AWS Bedrock, self-hosted) is expected to change over time as the project evolves and as costs and privacy requirements shift.

## Decision

Define `Categorizer` as a Go interface from day one. Phase 1 ships with `AnthropicCategorizer` (Claude Haiku via the Anthropic API).

## Rationale

- **Planned evolution:** Phase 2 targets AWS Bedrock (Llama/Mistral) to keep data within AWS. Phase 3 may use a self-hosted model on an on-demand GPU for full data privacy. Each is a different implementation of the same interface.
- **Privacy:** financial data leaving the machine to a third-party API is a valid concern. A swappable interface lets us move to Bedrock or a local model without rewriting any calling code.
- **Testability:** the interface makes it trivial to inject a mock in tests, avoiding real API calls and costs during development.
- **Go idiom:** interfaces defined by the consumer (not the implementer) is standard Go design.

## Consequences

- All callers depend on the interface, never on a concrete type. New implementations only need to satisfy the interface.
- The interface contract must be stable. Adding methods later is a breaking change for all implementations.
- Phase 2 requires Bedrock SDK integration and IAM permissions for `bedrock:InvokeModel` — Terraform changes, not code changes.
