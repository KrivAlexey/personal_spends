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
`internal/logging` provides `WithAttrs(ctx, attrs...) context.Context` and a handler wrapper whose `Handle` appends the attributes stored in the context to the record. The wrapper also overrides `WithAttrs` and `WithGroup` to return a wrapper around the derived inner handler, so `logger.With(...)` and groups keep the context attributes. `logging.WithAttrs` replaces an attribute whose key is already in the context instead of appending it, so the second batch's context holds `batch=2` once, not `batch=1` and `batch=2`. Callers always use the `…Context` methods (`DebugContext`, `ErrorContext`).
- The middleware stores `request_id`.
- The importer stores `batch` and `batches` around each categorize-and-save step.

This lets each `Categorizer` write the batch record the `categorizer` spec asks for (`categorizer.Claude` first), with its own `size`, `duration_ms`, `input_tokens`, `output_tokens` and `stop_reason` plus `batch`/`batches`/`request_id` from the context, without the interface changing or the importer seeing token counts.
*Alternative:* two records per batch, one from the importer (position, duration) and one from Claude (tokens). Rejected because the spec wants one record per batch to read in a log viewer.

### Claude logs before unpacking
The batch DEBUG record is written right after `Messages.New` returns and before the tool output is validated. A `max_tokens` response that then fails validation still shows `stop_reason=max_tokens` next to the ERROR record (#65).

### Log once, at the boundary
Lower layers return wrapped errors and never log them. `uploadCSV` writes the single ERROR record on 500.

A client that disconnects isn't a server fault, so `uploadCSV` logs at WARN when `errors.Is(err, context.Canceled) && errors.Is(r.Context().Err(), context.Canceled)`. The second check makes sure the request's own context was cancelled, not merely done (`Err() != nil` is also true after its deadline): a `context.Canceled` from some internal context we cancelled ourselves would be a bug and stays ERROR. `context.DeadlineExceeded` also stays ERROR, because a timeout is ours to fix. The response doesn't change; the client is gone and never reads it. The one exception is the store's retry WARN: a retry that succeeds returns no error, so it would otherwise be invisible.

### Middleware owns the request ID and access record
`Handler.Routes()` wraps the mux in a middleware that:
1. generates `request_id` (16 hex characters from `crypto/rand`) and stores it in the context;
2. wraps `ResponseWriter` to capture the status;
3. writes the INFO access record when the handler returns, with `route` read from `r.Pattern`. `ServeMux` sets `Pattern` on the request it is handed, which is the copy made by `r.WithContext` for `request_id`. The middleware keeps that copy in a variable and reads `Pattern` from it after `next.ServeHTTP`; reading the original request would give `""` and log every route as `unmatched`. It is empty for a 404, logged as `unmatched`.

The access record and panic handling run in a `defer`, so they happen however `next.ServeHTTP` exits. Code placed after the call would be skipped by a panic, which unwinds straight through the middleware to `net/http`'s own recover: that prints plain text to stderr without `request_id` and drops the connection, so the client gets no response at all. #36 (index out of range on bad Claude output) was this kind of panic, fixed before the endpoint existed. The deferred function:
1. calls `recover()`. On a panic other than `http.ErrAbortHandler` it writes the ERROR record with `stack` from `debug.Stack()`. It logs the stack, not the panic value, because the value is free text that could carry bank data.
2. on such a panic, if the status writer has sent nothing yet, writes `500 internal error`. A deliberate `http.ErrAbortHandler` skips steps 1 and 2: the handler chose to drop the connection, so it gets no 500 and no ERROR record.
3. writes the INFO access record with the status the client actually received: `500` after step 2, the already-sent status (e.g. `200`) for a panic after the response started, and `0` for a deliberate abort before anything was sent.
4. only then, if the response had already started (or the handler aborted on purpose with `http.ErrAbortHandler`), re-panics with `http.ErrAbortHandler`. `net/http` treats that as a silent abort and closes the connection. The re-panic must come last: it leaves the deferred function at once, so any step after it would never run.

The status writer counts the response as started on the first `WriteHeader` or `Write` (a `Write` without `WriteHeader` sends an implicit `200`). It implements `Unwrap() http.ResponseWriter`, so `http.ResponseController` still reaches `Flusher` and `Hijacker` on the underlying writer.

Panics in goroutines a handler starts, process kills and Lambda timeouts are out of reach of any `recover` in the middleware. A DEBUG "request started" record would leave a trace for those; revisit when `cmd/lambda` lands.

The raw path is never logged: it is caller input of any length and could carry bank data (`/REWE-23,45`). The route pattern is fixed text from our code.

`bank` comes from the caller, so every record cuts it to 64 runes plus `…` (counted in runes, not bytes, so a multi-byte character such as `ä` is never split), and the 400 `reason` is fixed text without it. The default 1 MB header limit would otherwise allow a 1 MB value per record.

The incoming `X-Request-Id` is ignored for now. On Lambda the API Gateway request ID can replace the generated one when `cmd/lambda` lands, with no change to the spec.

### Level and format parsing in `internal/logging`
`logging.New(w io.Writer, level, format string) (*slog.Logger, error)` parses with `slog.Level.UnmarshalText` (it already accepts `debug|info|warn|error`, case-insensitive) and picks `slog.NewTextHandler` or `slog.NewJSONHandler`. `cmd/*` passes `os.Getenv` values and exits on error. Tests call it with a `bytes.Buffer`.

### Bank-data safety lives in the error messages
The log call sites only ever log counts, so the risk is the `error` field. The fix is at the source:
- `storage`: `can't marshal map Expense:%v` / `can't write Expense:%v` → identify the expense by its `Date` and its position in the batch only. Transaction IDs stay out of logs: they are hashes of row content (see Risks).
- `parser`: `parsing column %q: %w` wraps strconv/time errors that quote the raw value → return a typed `*parser.ParseError{Column string; Row int}` whose `Error()` names the column and row but never the value, and which does not wrap the `strconv`/`time` error. Callers tell parse failures from I/O errors with `errors.As(err, &*parser.ParseError)`, which is what #91 needs to answer them with 400. I/O and sentinel errors keep `%w`. The `csv-import` spec doesn't describe parse-error content, so it needs no delta.
- `categorizer`: the existing messages hold indices and counts only. The SDK's `create message` error holds the HTTP status and API error body, not our prompt (to be confirmed, see Risks).

## Risks / Trade-offs

- [A future error message embeds content again] → the spec's "Debug upload of a real-looking export" scenario is a handler-level test that runs a full fake import at DEBUG and greps the buffer for the fixture's merchant, description and amount. A regression fails it.
- [SDK error text could echo request content] → assumed not, for both the Anthropic SDK and the AWS SDK (DynamoDB errors such as `ValidationException` can quote attribute values). Checked as a merge gate: the implementation PR doesn't merge until task 3.8 confirms it from both SDKs' sources, or the error is stripped where it enters our code.
- [Every `Categorizer` must write the batch record itself] → the `categorizer` spec makes it a contract, with `0`/`none` for implementations that call no model. The importer tests' fake `Categorizer` meets it too, so the two-batch scenario is tested at the importer level. Every implementation's tests MUST call the shared contract helper from task 2.4; a new implementation without that call is incomplete. `BedrockCategorizer` gets the same test as `Claude`.
- [Context attributes are invisible at call sites] → limited to `request_id`, `batch`, `batches`; documented in the package.
- [Transaction IDs are hashes of row content] → not logged at all. An ID is the first 64 bits of a SHA-256 over the identity columns; for Sparkasse that's 16 columns including IBAN and references, hard to guess back, but a bank with few guessable identity columns (date and amount only) would not be. No record needs an ID today, so the spec leaves them off the allowed list; adding one later is a spec change.
- [Bank-data tests miss a reformatted value] → the scenarios match case-insensitively and check each amount in raw (`23,45`) and parsed (`23.45`) form.
- [Text format on Lambda is hard to query] → Terraform sets `LOG_FORMAT=json` when the Lambda lands.

## Migration Plan

None. Additive, with no stored data or API change. Rollback is a revert.
