Test-first per CLAUDE.md: each group commits its failing table tests before the implementation. Test fixtures use made-up account numbers, references and merchants, never real export data.

## 1. Mapping settings

- [x] 1.1 Failing tests for mapping validation: valid Sparkasse entry; empty `identityColumns`; duplicate identity column; only `pendingColumn` or only `pendingValue` set; no pending settings (valid)
- [x] 1.2 Add `PendingColumn`, `PendingValue`, `IdentityColumns` to `BankMapping` and validate them in `applyDefaults`
- [x] 1.3 Add Sparkasse's `pendingColumn: Info`, `pendingValue: "Umsatz vorgemerkt"` and `identityColumns` (every column except `Info` and `Kategorie`) to `bank_mappings.yaml`; extend `TestFileMappingProvider_CommittedMappings` to cover them

## 2. Parser: pending filter and identity

- [x] 2.1 Failing table tests for `ParseCSV` with an anonymized 18-column Sparkasse fixture covering: pending rows skipped and counted; booked row parsed with booking date; identical ID for the same row across two parses; IDs differ when only `Auftragskonto` differs; IDs equal when only `Kategorie` or `Info` differs; whitespace-only differences ignored; two identical rows get `-0` and `-1`; missing identity column fails before any row; bank without a pending marker imports all rows
- [x] 2.2 Add `ID` to `categorizer.Transaction`
- [x] 2.3 Change `ParseCSV` to return `ParseResult{Transactions, PendingSkipped}`; resolve identity and pending column indexes from the header; compute `sha256(trimmed cells joined by \x1f)[:16] + "-" + n` with a per-call occurrence counter

## 3. Storage: known-ID lookup

- [ ] 3.1 Failing tests with the fake `dynamoDBAPI`: empty input; mix of known and unknown keys; more than 100 keys split into chunks; `UnprocessedKeys` retried, then an error after `MaxRetryAttempts`; client error wrapped; context cancelled
- [ ] 3.2 Add `BatchGetItem` to `dynamoDBAPI` and implement `KnownIDs(ctx, []ExpenseKey) (map[string]bool, error)` with SK-only projection and the shared retry/backoff

## 4. Ingest package

- [ ] 4.1 Failing table tests for `Importer.Import` with fake parser, categorizer and store: all new rows; all known (no `Categorize` call, no save); mixed known and new (only new rows categorized and saved); pending count passed through; counts add up to the row total; batches of `batchSize`, including a partial last batch; categorizer error on batch N leaves batches before N saved and returns the error; `CreatedAt` from the injected clock
- [ ] 4.2 Create `internal/ingest` with consumer-side `parser`/`store` interfaces, `Importer`, `Result` and `toExpense`; serial batch loop per ADR 0007

## 5. Docs and verification

- [ ] 5.1 Update `docs/architecture.md`: Implementation Status (`ingest`, `KnownIDs`), and the parser line in Core Interfaces if its signature is shown
- [ ] 5.2 `go build ./... && go vet ./... && go test ./...`; re-run `openspec validate idempotent-csv-upload --strict`
- [ ] 5.3 Archive the change once implemented (`openspec archive idempotent-csv-upload`), creating `openspec/specs/csv-import/spec.md`
