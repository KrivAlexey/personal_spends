# 0013 — Structured logging with `log/slog`, injected and bank-data free

**Status:** Accepted

## Context

`uploadCSV` answered every import failure with a generic `500 internal error` and discarded the error (#88), so a DynamoDB or Claude failure was recorded nowhere. Normal processing was invisible too: no row counts, batch timings or `max_tokens` stops (#65). CLAUDE.md forbids global state. The logs end up in CloudWatch once the Lambda lands, and the uploads are bank exports.

## Decision

- **`log/slog`, standard library only.** `logging.New(w, level, format)` builds the one logger from `LOG_LEVEL` (`debug|info|warn|error`, default `info`) and `LOG_FORMAT` (`text|json`, default `text`); an unknown value is an error naming the variable, and `cmd/*` stops startup on it.
- **Injected, not global.** `handler.New`, `ingest.NewImporter`, `categorizer.NewClaude` and `storage.NewStore` take a `*slog.Logger`; nil means discard.
- **Correlation fields come from the context.** `logging.WithAttrs(ctx, …)` stores `request_id` (set by the HTTP middleware) and `batch`/`batches` (set by the importer); a `slog.Handler` wrapper adds them to every record logged with a `…Context` method.
- **Log once, at the boundary.** Lower layers return wrapped errors and never log them. `uploadCSV` writes the single ERROR record on `500`, WARN when the client disconnected. The store's retry WARN is the one exception, because a retry that succeeds returns no error.
- **The `Categorizer` contract changes, not its signature.** Every implementation writes one DEBUG record per batch with `size`, `duration_ms`, `input_tokens`, `output_tokens` and `stop_reason` (owner-approved 2026-10-24: https://github.com/KrivAlexey/personal_spends/pull/105#discussion_r4367519063).
- **No bank data in logs, at any level.** No request body, CSV cell, merchant, description, amount, prompt or response text, transaction ID or secret. Error messages that embedded such data (`storage`'s `%v` of an `Expense`, parser errors quoting the cell) are changed at the source, so the error chain is safe to log. SDK errors are stripped where they enter our code, because their text holds server-written messages we can't vouch for (the Anthropic SDK appends the raw response body, the AWS SDK the API error message): only the status, error type/code and request ID pass through. The raw request path is never logged, only the matched route; a caller's `bank` value is cut to 64 runes.

The record set and fields are specified in [`openspec/specs/logging`](../../openspec/specs/logging/spec.md).

## Alternatives considered

| Option | Rejected because |
|---|---|
| `slog.Default()` | Global state, against CLAUDE.md |
| Logger carried in the context | Hides a dependency; the classic `context.Value` misuse |
| zap / zerolog | A new dependency for nothing slog lacks here |
| Importer and categorizer each write a record per batch | Two records to join for one batch; the wrapper lets one record carry both position and token counts without the interface changing |

## Consequences

- Every new `Categorizer` must write the batch record and call the shared contract test helper in its tests.
- Every new error message must keep bank data out; the handler-level bank-data test catches a regression.
- `cmd/*` and Terraform set `LOG_LEVEL`/`LOG_FORMAT` (`json` on Lambda) when they land. MCP server logging goes to stderr, decided when `internal/mcp` lands.
