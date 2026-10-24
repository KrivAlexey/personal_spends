## Why

`uploadCSV` answers every import failure with a generic `500 internal error` and discards the error (#88). A DynamoDB or Claude failure is recorded nowhere, so it can't be debugged, especially on Lambda. Normal processing is just as invisible: nothing shows how many rows were parsed, how long a Claude batch took, or whether a batch hit `max_tokens` (#65).

## What Changes

- Structured logging with `log/slog`. A single logger is built from `LOG_LEVEL` (`debug|info|warn|error`, default `info`) and `LOG_FORMAT` (`text|json`, default `text`) and injected into the handler, importer, categorizer and store.
- Every HTTP request gets a request ID, and every record logged while serving the request carries it.
- Every request logs one INFO access record: `method`, `route` (the matched route pattern, never the raw path), `status`, `duration_ms`.
- A 500 logs one ERROR record with `method`, `route`, `bank` and the full wrapped error chain. The response body stays generic.
- A 400 logs a WARN record with a fixed-text reason. A caller-supplied `bank` value is cut to 64 runes (Unicode characters) in every record.
- DEBUG records trace normal processing with counts, timings and token usage: rows parsed, pending skipped, already known, each Claude batch (size, duration, input/output tokens, stop reason), each save, and the import summary.
- DynamoDB retry attempts log at WARN.
- Logs never contain bank data: no request body, CSV row, merchant, description, amount, or Claude prompt/response text, at any level. Error messages that embed such data today (`storage` `%v` of an `Expense`, parser errors quoting raw cell values) are changed so the error chain is safe to log.
- `openspec/config.yaml` scope widens to include operational logging. Logs are observable behavior for the operator.

## Capabilities

### New Capabilities
- `logging`: what is logged at each level, which fields every record carries, request correlation, level/format configuration, and the rule that bank data never reaches the logs.

### Modified Capabilities
- `categorizer`: every `Categorizer` implementation writes one DEBUG record per batch (size, duration, token usage, stop reason). This changes the core interface's contract, not its method signature. Approved by the owner on 2026-10-24: https://github.com/KrivAlexey/personal_spends/pull/105#discussion_r4367519063

`rest-api` responses are unchanged: a 500 still returns `internal error` with no details. Parse failures stay `500` here; #91 moves them to `400`.

## Impact

- New `internal/logging` package: logger construction from level/format, request-ID context helpers, and a `slog.Handler` wrapper that adds `request_id` from the context.
- Constructor signatures gain a `*slog.Logger`: `handler.New`, `ingest.NewImporter`, the Claude categorizer, the DynamoDB store. A nil logger means discard, so existing tests don't need one. These are not core interfaces. The `Categorizer` method signature is unchanged; only its contract changes (see Modified Capabilities).
- `internal/handler`: request middleware (request ID + access record), ERROR on 500, WARN on 400.
- `internal/storage`, `internal/parser`: error messages stop embedding expense and cell content.
- Docs: new ADR (slog, injection, format, log-once rule, no bank data), `docs/architecture.md` Configuration table (`LOG_LEVEL`, `LOG_FORMAT`) and a short Logging section.
- Out of scope: `cmd/*` wiring (doesn't exist yet; it calls `logging.New` when it lands and stops startup on its error), Lambda/Terraform env vars, metrics and tracing, MCP server logging (stdout is the protocol channel there, so it will need stderr, decided when `internal/mcp` lands).
- No new dependencies.
