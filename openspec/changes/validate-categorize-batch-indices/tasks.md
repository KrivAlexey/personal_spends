## 1. Validate tool output

- [x] 1.1 In `unpackCategorizeBatch` (internal/categorizer/claude.go), after unmarshaling `result.Results`, validate before building `CategorizedTransaction` values: every `Index` is within `[0, len(transactions))`, and the set of indices covers `[0, len(transactions))` exactly once (no duplicate, none missing).
- [x] 1.2 On validation failure, return `nil, fmt.Errorf("categorizer: ...")` describing the specific problem (out-of-range index, duplicate index, or missing index) instead of indexing into `transactions`.
- [x] 1.3 On success, build `[]CategorizedTransaction` in input order (index 0..len(transactions)-1), same as today.

## 2. Tests

- [x] 2.1 Add table-driven unit tests for `unpackCategorizeBatch` using a fake `*anthropic.Message` with a `categorize_batch` tool-use block, covering: valid full-coverage result, out-of-range index, duplicate index, missing index.
- [x] 2.2 Run `go test ./internal/categorizer/...` and confirm the new tests pass and the existing skipped integration test is unaffected.
