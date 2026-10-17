## Why

`ingest.Importer.Import` is built and tested, but nothing calls it over HTTP, and `POST /uploads/csv` had no documented way to name the bank (#77). ADR 0012 settles the request shape; this change specifies the observable contract the handler must meet.

## What Changes

- `POST /uploads/csv?bank=<bankName>` accepts a bank export as the raw request body and imports it.
- Success returns `200` with JSON `{"imported", "already_known", "pending_skipped"}`.
- A missing or empty `bank`, an empty body, or an unknown bank returns `400`. Nothing is imported.
- Any other import failure returns `500`. The response does not include internal error details.
- The body reaches the import byte for byte; decoding stays with the bank mapping (`csv-import`).

## Capabilities

### New Capabilities
- `rest-api`: the HTTP contract of the REST API, starting with `POST /uploads/csv`. Later endpoints (`/expenses`, `/categories`) add requirements here.

### Modified Capabilities
None. `csv-import` requirements (dedup, pending rows, result counts, encoding) are unchanged; the endpoint exposes them.

## Impact

- New `internal/handler` package (`Handler`, `Routes`, `uploadCSV`) and its tests (failing tests already committed on `feature/upload-csv-handler`).
- `docs/architecture.md` upload flow and API table, ADR 0012 (already committed).
- Out of scope: `X-API-Key` middleware, `cmd/server` / `cmd/lambda` wiring, an upload size limit, a 4xx for exports missing identity columns (the parser has no sentinel error for it yet).
- No new dependencies; no core interface changes.
