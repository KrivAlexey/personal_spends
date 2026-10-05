# 0007 — Serial batch loop for v1, worker pool in step 2

**Status:** Accepted. Defers [0003](0003-worker-pool-for-csv.md) to step 2.

## Context

[0003](0003-worker-pool-for-csv.md) chose a goroutine worker pool to categorize CSV batches concurrently. In v1, the first CSV upload is also the first time handler, parser, categorizer and storage run together. Any failure on that path could come from any of the four, and concurrency would add out-of-order results and partial failures on top.

## Decision

v1 walks the ~50-transaction batches in a plain serial loop: categorize one batch, then the next, then save. The worker pool from 0003 replaces the loop as step 2, the first backlog item once v1 runs end to end.

## Rationale

- **Debuggability:** a serial loop has one order of events, and stack traces and logs read top to bottom. That matters most while the path is being wired up for the first time.
- **Nothing is lost:** 0003's design still holds. The loop and the pool share the same `Categorizer` call per batch, so the pool is a swap-in, not a rewrite.
- **Learning order:** the concurrency exercise is clearer against a working serial baseline, where the speedup and the new failure modes can be measured.

## Consequences

- v1 uploads take roughly (number of batches) × (one Claude call, ~1–2 s). A few hundred rows finishes well within Lambda's limit.
- `WORKER_POOL_SIZE` is documented but unused until step 2.
- Step 2 must keep the serial loop's behavior on partial failure, or change it deliberately, with a test for each case.
