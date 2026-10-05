## Why

Bank exports overlap ("last month", "last 90 days"), and the same file can be uploaded twice. Today nothing identifies a transaction across uploads, so every overlapping row would be stored again and every spending summary inflated. Sparkasse exports also contain pending rows whose merchant and description are placeholders that change once booked, so importing them adds junk now and a duplicate later. No IDs are stored yet, so the identity scheme can be fixed now without migrating data. The decision and the alternatives are in ADR 0011.

## What Changes

- Pending rows (per the bank's mapping, e.g. Sparkasse `Info = "Umsatz vorgemerkt"`) are skipped and counted, never imported.
- Every imported transaction gets a deterministic identity derived from the bank's own text in the bank's configured identity columns, plus an occurrence counter for fully identical rows in the same export.
- Importing an export skips transactions already stored: they are not categorized again and not rewritten.
- An import reports how many rows were imported, already known, and skipped as pending.
- The expense date is the booking date for every row.
- Bank mappings gain three settings: the pending column and value, and the identity columns. An export missing a configured identity column is rejected.

## Capabilities

### New Capabilities
- `csv-import`: importing a bank CSV export into stored expenses: which rows are imported, how a transaction is recognized across exports, and what the import reports.

### Modified Capabilities
(none — `categorizer`'s requirements are unchanged; it receives only new rows)

## Impact

- `internal/parser`: mapping settings (`pendingColumn`, `pendingValue`, `identityColumns`) and their validation; parsing drops pending rows, rejects missing identity columns, and assigns each transaction its ID.
- `internal/parser/bank_mappings.yaml`: Sparkasse gets its pending marker and identity columns.
- `internal/categorizer`: `Transaction` carries the ID through categorization.
- `internal/storage`: a lookup of which IDs are already stored (DynamoDB `BatchGetItem`); `Expense.ID` is the transaction ID.
- New `internal/ingest`: the import loop (filter known → batch → categorize → save → counts). The future upload handler calls it, and the worker pool in step 2 replaces its serial loop.
- `docs/architecture.md`: CSV flow and data model (`SK = <date>#<id>`).
- No existing stored data: nothing to migrate.
