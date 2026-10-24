## Context

Nothing logs today. Errors are wrapped with `%w` at each layer (`ingest: categorize: categorizer: create message: …`), so the chain that reaches `uploadCSV` already tells the whole story; it is just discarded. `cmd/*` doesn't exist yet, so nothing reads configuration. CLAUDE.md forbids global state, so `slog.Default()` is out. The `Categorizer` interface returns only `[]CategorizedTransaction`: token usage and stop reason are visible only inside `categorizer.Claude`.

## Goals / Non-Goals

**Goals:**
- The `logging` spec, met with the standard library only.
- Each layer logs what only it knows, and correlation fields come from the context, not from extra parameters.

**Non-Goals:**
- Changing the `Categorizer` method signature to return usage. Its contract does change: every implementation logs its batches (`categorizer` spec delta, owner-approved 2026-10-24).
- Log sampling, metrics, tracing, Lambda log-group retention (Terraform, later).

## Decisions

### `log/slog`, injected `*slog.Logger`
Constructors take a `*slog.Logger`: `handler.New`, `ingest.NewImporter`, `categorizer.NewClaude`, `storage.NewStore`. A nil logger becomes `slog.New(slog.DiscardHandler)`, so tests that don't assert on logs pass nil.
*Alternatives:* `slog.Default()` (global state, against CLAUDE.md); a logger in the context (hides a dependency, and it's the classic `context.Value` misuse); zap/zerolog (new dependency, and nothing slog lacks here).

### Context attributes through one `slog.Handler` wrapper
`internal/logging` provides `WithAttrs(ctx, attrs...) context.Context` and a handler wrapper whose `Handle` appends the attributes stored in the context to the record. Callers always use the `…Context` methods (`DebugContext`, `ErrorContext`).
- The middleware stores `request_id`.
- The importer stores `batch` and `batches` around each categorize-and-save step.

This lets each `Categorizer` write the batch record the `categorizer` spec asks for (`categorizer.Claude` first), with its own `size`, `duration_ms`, `input_tokens`, `output_tokens` and `stop_reason` plus `batch`/`batches`/`request_id` from the context, without the interface changing or the importer seeing token counts.
*Alternative:* two records per batch, one from the importer (position, duration) and one from Claude (tokens). Rejected because the spec wants one record per batch to read in a log viewer.

### Claude logs before unpacking
The batch DEBUG record is written right after `Messages.New` returns and before the tool output is validated. A `max_tokens` response that then fails validation still shows `stop_reason=max_tokens` next to the ERROR record (#65).

### Log once, at the boundary
Lower layers return wrapped errors and never log them. `uploadCSV` writes the single ERROR record on 500. The one exception is the store's retry WARN: a retry that succeeds returns no error, so it would otherwise be invisible.

### Middleware owns the request ID and access record
`Handler.Routes()` wraps the mux in a middleware that:
1. generates `request_id` (16 hex characters from `crypto/rand`) and stores it in the context;
2. wraps `ResponseWriter` to capture the status;
3. writes the INFO access record when the handler returns.

`bank` comes from the caller, so every record cuts it to 64 characters plus `…`, and the 400 `reason` is fixed text without it. The default 1 MB header limit would otherwise allow a 1 MB value per record.

The incoming `X-Request-Id` is ignored for now. On Lambda the API Gateway request ID can replace the generated one when `cmd/lambda` lands, with no change to the spec.

### Level and format parsing in `internal/logging`
`logging.New(w io.Writer, level, format string) (*slog.Logger, error)` parses with `slog.Level.UnmarshalText` (it already accepts `debug|info|warn|error`, case-insensitive) and picks `slog.NewTextHandler` or `slog.NewJSONHandler`. `cmd/*` passes `os.Getenv` values and exits on error. Tests call it with a `bytes.Buffer`.

### Bank-data safety lives in the error messages
The log call sites only ever log counts, so the risk is the `error` field. The fix is at the source:
- `storage`: `can't marshal map Expense:%v` / `can't write Expense:%v` → identify the expense by `ID` and `Date` only.
- `parser`: `parsing column %q: %w` wraps strconv/time errors that quote the raw value → replace with a message naming the column and row number, without wrapping the value-bearing error (keep `%w` only for sentinel or I/O errors).
- `categorizer`: the existing messages hold indices and counts only. The SDK's `create message` error holds the HTTP status and API error body, not our prompt (to be confirmed, see Risks).

## Risks / Trade-offs

- [A future error message embeds content again] → the spec's "Debug upload of a real-looking export" scenario is a handler-level test that runs a full fake import at DEBUG and greps the buffer for the fixture's merchant, description and amount. A regression fails it.
- [Anthropic SDK error text could echo request content] → assumed not. Check the SDK's `Error.Error()` during implementation and list it under **Not verified** if unconfirmed.
- [Every `Categorizer` must write the batch record itself] → the `categorizer` spec makes it a contract, with `0`/`none` for implementations that call no model. The importer tests' fake `Categorizer` meets it too, so the two-batch scenario is tested at the importer level. `BedrockCategorizer` gets the same test as `Claude`.
- [Context attributes are invisible at call sites] → limited to `request_id`, `batch`, `batches`; documented in the package.
- [Bank-data tests miss a reformatted value] → the scenarios match case-insensitively and check each amount in raw (`23,45`) and parsed (`23.45`) form.
- [Text format on Lambda is hard to query] → Terraform sets `LOG_FORMAT=json` when the Lambda lands.

## Migration Plan

None. Additive, with no stored data or API change. Rollback is a revert.
