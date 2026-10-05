## Why

`Claude.Categorize` builds its result by indexing `transactions[r.Index]` directly from the `categorize_batch` tool-use JSON that Claude returns. That JSON is external, untrusted model output, not a value the Go code controls. An out-of-range `index`, or a `results` array whose length doesn't match the input batch, currently causes an unrecovered panic (`index out of range`) instead of a handled error, which can take down the request path processing that batch.

## What Changes

- Validate `categorize_batch` tool output in `unpackCategorizeBatch` before indexing into `transactions`: each `index` must be in `[0, len(transactions))`, and every input index must be covered exactly once (no missing, no duplicate).
- On validation failure, return a descriptive `error` (as the function already does for the "no tool call" case) instead of panicking.

## Capabilities

### New Capabilities
- `categorizer`: Categorizes bank transactions via an LLM tool-use call and turns the model's structured output into `CategorizedTransaction` values, validating that output against the input batch before use.

### Modified Capabilities
(none — no existing specs in this repo yet)

## Impact

- `internal/categorizer/claude.go`: `unpackCategorizeBatch` gains validation logic.
- `internal/categorizer/claude_test.go`: needs unit tests covering out-of-range, duplicate, and missing indices (currently the only test is a skipped live-API integration test).
- No API or storage changes; `Categorizer` interface signature is unchanged.
