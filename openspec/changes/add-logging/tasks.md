## 1. Decision and scope

- [ ] 1.1 ADR 0013: `log/slog` with an injected logger, context attributes via a handler wrapper, log once at the boundary, no bank data in logs, `LOG_LEVEL`/`LOG_FORMAT`. State explicitly that the `Categorizer` contract changes: every implementation logs one DEBUG record per batch (owner-approved 2026-10-24: https://github.com/KrivAlexey/personal_spends/pull/105#discussion_r4367519063)
- [ ] 1.2 Widen `openspec/config.yaml` context: OpenSpec also covers operational logging (what the operator can rely on in logs)
- [ ] 1.3 Note in `docs/architecture.md` (Interfaces) that every `Categorizer` logs one DEBUG record per batch

## 2. Failing tests (one commit, red for the right reason)

- [ ] 2.1 `internal/logging`: the context-attribute wrapper keeps context attributes through `logger.With(...)` and `WithGroup`; setting `batch` twice in a context yields one `batch` field with the latest value. Table test for `New` (valid level/format pairs, case-insensitive level, invalid `LOG_LEVEL`/`LOG_FORMAT` return an error naming the variable and value and a nil logger, JSON keys `time`/`level`/`msg`) and for context attributes appearing on records
- [ ] 2.2 `internal/handler`: with a JSON logger into a buffer, cover the spec's access-record, request-ID, 500-ERROR (exactly one record, `bank`, full `error`), 400-WARN (fixed `reason` without the `bank` value), a `bank` value with a newline and `level=ERROR` staying one escaped record in both formats, `route=POST /uploads/csv` for a successful upload (catches reading `Pattern` from the wrong request) and `unmatched` for an unknown path (no part of the path in the buffer), oversized-bank (cut to 64 runes plus `…`, including the `ä`×100 case with no U+FFFD) and default-level-stays-quiet scenarios
- [ ] 2.3 `internal/ingest`: at DEBUG, parse/dedup/save/summary records with the spec's fields, the two-batch scenario (`batch`/`batches` from context, with the fake `Categorizer` writing its batch record as the contract requires), and the nothing-new scenario
- [ ] 2.4 `internal/categorizer`: a shared contract test helper (e.g. `categorizertest.AssertBatchLogging`) that every `Categorizer` implementation's tests MUST call, the importer's fake included; a new implementation without that call is incomplete. Run it against `Claude` with the existing fake HTTP server for the `categorizer` spec's scenarios: batch record fields, `max_tokens` record written before the validation error, context attributes carried, no record when the API call fails
- [ ] 2.5 `internal/storage`: WARN retry record (`operation`, `attempt` counting retries from 1, `unprocessed`) via the fake `dynamoDBAPI`; write/marshal errors contain no merchant, description or amount
- [ ] 2.6 `internal/parser`: a malformed amount or date cell yields a `*ParseError` (found with `errors.As`) naming the column and row, without the raw value in either `12,3x` or `12.3x` form; an I/O error from the reader is not a `*ParseError`
- [ ] 2.7 Handler-level secrets test: the spec's "Anthropic key in an error path" scenario, with a sentinel key (`X-API-Key` waits for the auth middleware change)
- [ ] 2.8 Handler-level bank-data test: a full fake import at DEBUG with the spec's fixture values, asserting none of them appears in the buffer, case-insensitively and with each amount in raw and parsed form

## 3. Implementation

- [ ] 3.1 `internal/logging`: `New(w, level, format)`, `WithAttrs(ctx, …)` (replaces an existing key), and the context-attribute `slog.Handler` wrapper, overriding `WithAttrs` and `WithGroup` to stay wrapped
- [ ] 3.2 Add a `*slog.Logger` parameter (nil = discard) to `handler.New`, `ingest.NewImporter`, `categorizer.NewClaude`, `storage.NewStore`, and update callers and tests
- [ ] 3.3 Handler: middleware (request ID, status-capturing writer, INFO access record with `route` from the `r.WithContext` copy's `Pattern`), ERROR on 500, WARN on 400 with fixed `reason`, `bank` cut to 64 runes in every record
- [ ] 3.4 Importer: DEBUG records, plus `batch`/`batches` context attributes around each batch
- [ ] 3.5 Claude: time `Messages.New`, write the batch DEBUG record before `unpackCategorizeBatch`, none when `Messages.New` fails
- [ ] 3.6 Store: WARN on each retry in batch write and batch get, and drop `%v` of `Expense` from error messages
- [ ] 3.7 Parser: column-parse failures return `*ParseError{Column, Row}`, whose message drops the raw value and which doesn't wrap the `strconv`/`time` error
- [ ] 3.8 Merge gate: confirm from the Anthropic SDK source that its error text can't echo request content, and record the result in the PR. If it can, strip the echoed part in the categorizer before the PR merges
- [ ] 3.9 `go build ./... && go vet ./... && go test ./...` passes

## 4. Docs and ship

- [ ] 4.1 `docs/architecture.md`: `LOG_LEVEL`/`LOG_FORMAT` in Configuration, a short Logging section, and `internal/logging` in Implementation Status. Remove "logging of 500s (#88)" from the handler's "Not yet"
- [ ] 4.2 Open the PR for #88 (`Closes #88`), `ci` green
- [ ] 4.3 Archive this change once merged
