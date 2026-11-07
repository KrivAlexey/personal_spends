## Why

`BankMapping.applyDefaults` never checks the `mappings:` values, and the parser silently ignores any value outside `Date|Merchant|Amount|Currency|Description` (#99). A typo (`Datum: date`) or a missing `Date` stores every row with a zero date, key `0001-01-01#<id>`; a missing `Amount` stores every row as 0. A `bankName` listed twice silently replaces the earlier entry. These rows are hard to clean up because the date is part of the sort key.

## What Changes

- Loading the mapping file fails, naming the bank, when a `mappings:` value is not one of `Date`, `Merchant`, `Amount`, `Currency`, `Description`.
- Loading fails when a bank maps no column to `Date` or none to `Amount`.
- Loading fails when two columns map to the same field.
- Loading fails when a `bankName` appears twice.
- An export whose header lacks a column named in `mappings` is rejected, naming the column (a misspelled key like `Buchungstg: Date` otherwise gives the same zero dates). Found in review of #113.

## Capabilities

### New Capabilities
None.

### Modified Capabilities
- `csv-import`: adds "Bank mappings are validated on load" and "Exports must contain the mapped columns".

## Impact

- `internal/parser/mapping.go` (`applyDefaults`, `NewFileMappingProvider`), `internal/parser/parser.go` (`newLayout`) and their tests.
- The committed `bank_mappings.yaml` already satisfies the new rules.
- No new dependencies; no core interface changes.
