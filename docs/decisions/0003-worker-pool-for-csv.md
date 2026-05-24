# 0003 — Worker pool for CSV processing

**Status:** Accepted

## Context

A bank CSV export can contain hundreds of transactions. Categorizing them sequentially (one Claude API call per batch, waiting for each to finish) is slow — Claude Haiku takes ~1–2 seconds per call.

## Decision

Use a worker pool of goroutines to process CSV batches concurrently within a single Lambda invocation.

## Rationale

- **I/O-bound workload:** each batch waits on a network call to the Claude API. Goroutines block cheaply during I/O, so running 5 concurrent batches reduces wall-clock time by ~5× with no added CPU cost.
- **Go learning:** goroutines + channels for fan-out/fan-in is the idiomatic Go concurrency pattern. This is a genuine use case, not a contrived exercise.
- **Rate limit awareness:** pool size (default 5) respects Claude API rate limits. Configurable via `WORKER_POOL_SIZE`.

## Consequences

- Concurrency adds complexity: results arrive out of order and must be collected via a results channel. Error handling needs to account for partial failures.
- Phase 2 replaces this pattern: when SQS fan-out is added, each Lambda invocation processes a single SQS batch sequentially. The goroutine concurrency moves to parallel Lambda invocations instead.
- Pool size tuning may be needed depending on Claude tier rate limits.
