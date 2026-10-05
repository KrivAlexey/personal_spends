# 0004 — Categorizer as a swappable interface

**Status:** Accepted

## Context

The categorization logic needs to call an AI model. The choice of model (Anthropic API, AWS Bedrock, self-hosted) is expected to change over time as the project evolves and as costs and privacy requirements shift.

## Decision

Define `Categorizer` as a Go interface from day one. v1 ships with `Claude` (`internal/categorizer/claude.go`, Claude Haiku via the Anthropic API).

## Rationale

- **Planned evolution:** the Bedrock backlog item targets AWS Bedrock (Llama/Mistral) to keep data within AWS. The on-demand GPU backlog item may use a self-hosted model for full data privacy. Each is a different implementation of the same interface.
- **Privacy:** financial data leaving the machine to a third-party API is a valid concern. A swappable interface lets us move to Bedrock or a local model without rewriting any calling code.
- **Testability:** the interface makes it trivial to inject a mock in tests, avoiding real API calls and costs during development.
- **Go idiom:** Go usually defines interfaces where they're consumed. A domain abstraction with several planned implementations is the standard exception (like `io.Reader`): `Categorizer` lives in `internal/categorizer` next to the `Transaction` types every implementation shares.

## Consequences

- All callers depend on the interface, never on a concrete type. New implementations only need to satisfy the interface.
- The interface contract must be stable. Adding methods later is a breaking change for all implementations.
- Bedrock needs a new `Categorizer` implementation using the Bedrock SDK, plus IAM permissions for `bedrock:InvokeModel` in Terraform. Callers don't change.
