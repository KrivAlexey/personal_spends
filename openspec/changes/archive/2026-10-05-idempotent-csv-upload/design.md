## Context

The decision itself (identity scheme, booked rows only, skip known rows, booking date) and the alternatives are recorded in [ADR 0011](../../../docs/decisions/0011-transaction-identity-and-deduplication.md). This design covers where each piece lives and how the pieces connect.

Current state:
- `parser.ParseCSV` maps header columns to `categorizer.Transaction` fields and returns `[]Transaction`. Unmapped columns are ignored, and the raw cells are dropped after parsing.
- `storage.Store` writes via `BatchWriteItem` with retries. It has no read path, and `Expense.ID` is set by nobody.
- No code connects parser, categorizer and storage; `cmd/` and `handler` don't exist.

## Goals / Non-Goals

**Goals:**
- Identity is assigned where the raw cells still exist: in the parser, in one pass over the whole file.
- The loop that connects parser, categorizer and storage lives in its own package, testable with fakes, so the step-2 worker pool replaces exactly that loop.

**Non-Goals:**
- The HTTP endpoint, the bank-name request parameter, and `cmd/` wiring (handler change).
- Deleting or correcting stored expenses (ADR 0011 consequence: done by hand).
- Using Sparkasse's `Kategorie` as a categorization hint.

## Decisions

### 1. The parser computes identity and filters pending rows

```
ParseCSV(ctx, bank, r) (ParseResult, error)

ParseResult {
    Transactions   []categorizer.Transaction   // booked rows only, ID set
    PendingSkipped int
}
```

- After reading the header, `ParseCSV` resolves `identityColumns` and `pendingColumn` to column indexes. A missing identity column is an error before any row is read.
- Per row: if `row[pendingIdx] == pendingValue`, count it and continue. Otherwise build the identity from the trimmed raw cells at the identity indexes, joined with `\x1f`, then sha256 and the first 16 hex characters.
- An occurrence counter `map[string]int` keyed by that base hash appends `-n`. It lives for one `ParseCSV` call, so the count spans the whole file.
- `categorizer.Transaction` gains `ID string`. It's the shared domain type already passed from parser to categorizer to storage; `CategorizedTransaction` embeds it, so the ID flows through categorization untouched.

*Alternative:* return raw cells and hash in `ingest`. Rejected: every caller would have to keep the raw cells and know the bank's identity columns, while the parser already owns the mapping.

### 2. Mapping settings, validated at load

```yaml
pendingColumn: Info                 # optional; both or neither
pendingValue: "Umsatz vorgemerkt"
identityColumns: [Auftragskonto, …] # required, non-empty
```

`applyDefaults` (already the load-time validator) rejects: an empty `identityColumns`, duplicate identity columns, and only one of `pendingColumn`/`pendingValue` set. A bad mapping fails at startup, as delimiters do today.

### 3. Storage answers "which of these are already stored?"

```
KnownIDs(ctx, keys []ExpenseKey) (map[string]bool, error)
ExpenseKey { Date time.Time; ID string }   // → PK "EXPENSES", SK "<date>#<id>"
```

- `BatchGetItem` in chunks of 100 keys (the DynamoDB limit), projecting only `SK`. `UnprocessedKeys` are retried with the same backoff as `writeChunkWithRetries`, so `dynamoDBAPI` gains `BatchGetItem`.
- `newExpenseItem` keeps its SK format; `Expense.ID` is now always the transaction ID.

*Alternative:* conditional `PutItem` (`attribute_not_exists(SK)`) per row. Rejected: it only tells you a row was known *after* categorizing it, so the Claude calls are already spent.

### 4. `internal/ingest` runs the loop

```
Importer{ parser, categorizer, store, batchSize, now }
Import(ctx, bank, r) (Result, error)

  ParseCSV → KnownIDs → drop known → batches of batchSize → Categorize → toExpense → SaveExpenses
  Result{ Imported, AlreadyKnown, PendingSkipped }
```

- The dependencies are interfaces. `parser` and `store` are unexported interfaces defined in `ingest` (narrow, consumer-side); `categorizer.Categorizer` is the domain interface.
- `toExpense` maps a `CategorizedTransaction` to a `storage.Expense`. `CreatedAt` comes from `now()`, which tests replace with a fixed time.
- Serial loop per ADR 0007. Batches are saved one at a time, after each is categorized: if batch 4 of 10 fails, batches 1–3 stay stored and the error is returned. The next import of the same file skips them as known, so retrying is safe.

## Risks / Trade-offs

- [The identity columns chosen for Sparkasse include a column the bank changes on booked rows] → every re-import shows `imported > 0`. Mitigation: the import counts make it visible, and the column comes out of the list.
- [Changing `ParseCSV`'s return type breaks callers] → there are none outside the package yet.
- [Partial failure leaves a half-imported file] → acceptable, because a retry is idempotent by design.
- [`KnownIDs` reads every row of an export] → billed per item read, not reduced by the projection: about 0.5 read units per key with eventually consistent reads, so a 500-row export is about 250 read units. Negligible at personal scale on on-demand billing.

## Migration Plan

None: nothing stores expenses yet.
