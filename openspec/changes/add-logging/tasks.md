## 1. Decision and scope

- [ ] 1.1 ADR 0013: `log/slog` with an injected logger, context attributes via a handler wrapper, log once at the boundary, no bank data in logs, `LOG_LEVEL`/`LOG_FORMAT`
- [ ] 1.2 Widen `openspec/config.yaml` context: OpenSpec also covers operational logging (what the operator can rely on in logs)

## 2. Failing tests (one commit, red for the right reason)

- [ ] 2.1 `internal/logging`: table test for `New` (valid level/format pairs, case-insensitive level, invalid `LOG_LEVEL`/`LOG_FORMAT` errors naming the variable and value, JSON keys `time`/`level`/`msg`) and for context attributes appearing on records
- [ ] 2.2 `internal/handler`: with a JSON logger into a buffer, cover the spec's access-record, request-ID, 500-ERROR (exactly one record, `bank`, full `error`), 400-WARN (`reason`) and default-level-stays-quiet scenarios
- [ ] 2.3 `internal/ingest`: at DEBUG, parse/dedup/save/summary records with the spec's fields, the two-batch scenario (`batch`/`batches` from context), and the nothing-new scenario
- [ ] 2.4 `internal/categorizer`: batch record from a canned `anthropic.Message` with usage and `stop_reason=max_tokens`, written even when unpacking then fails (test via an extracted helper, not the live API)
- [ ] 2.5 `internal/storage`: WARN retry record (`operation`, `attempt`, `unprocessed`) via the fake `dynamoDBAPI`; write/marshal errors contain no merchant, description or amount
- [ ] 2.6 `internal/parser`: a malformed amount cell yields an error naming the column, without the raw value
- [ ] 2.7 Handler-level bank-data test: a full fake import at DEBUG with the spec's fixture values, asserting none of them appears in the buffer

## 3. Implementation

- [ ] 3.1 `internal/logging`: `New(w, level, format)`, `WithAttrs(ctx, …)`, and the context-attribute `slog.Handler` wrapper
- [ ] 3.2 Add a `*slog.Logger` parameter (nil = discard) to `handler.New`, `ingest.NewImporter`, `categorizer.NewClaude`, `storage.NewStore`, and update callers and tests
- [ ] 3.3 Handler: middleware (request ID, status-capturing writer, INFO access record), ERROR on 500, WARN on 400
- [ ] 3.4 Importer: DEBUG records, plus `batch`/`batches` context attributes around each batch
- [ ] 3.5 Claude: time `Messages.New`, write the batch DEBUG record before `unpackCategorizeBatch`
- [ ] 3.6 Store: WARN on each retry in batch write and batch get, and drop `%v` of `Expense` from error messages
- [ ] 3.7 Parser: column-parse errors name the column and row and drop the raw value
- [ ] 3.8 Check whether the Anthropic SDK error text can echo request content; note the result in the PR's **Not verified** section if it can't be confirmed
- [ ] 3.9 `go build ./... && go vet ./... && go test ./...` passes

## 4. Docs and ship

- [ ] 4.1 `docs/architecture.md`: `LOG_LEVEL`/`LOG_FORMAT` in Configuration, a short Logging section, and `internal/logging` in Implementation Status. Remove "logging of 500s (#88)" from the handler's "Not yet"
- [ ] 4.2 Open the PR for #88 (`Closes #88`), `ci` green
- [ ] 4.3 Archive this change once merged
