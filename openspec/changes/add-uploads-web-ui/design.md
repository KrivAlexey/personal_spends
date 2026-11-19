# Design

## Context

- `internal/handler` serves exactly the patterns in `Handler.routes()`, and `openapi_test.go` fails when they differ from `openapi.yaml` ([0014](../../../docs/decisions/0014-hand-written-openapi-spec.md)).
- The upload path goes through `uploads.Service` and the Postgres `uploads` table ([0015](../../../docs/decisions/0015-upload-files-and-records.md), PR #124, not merged yet).
- Bank names live in the concrete `parser.FileMappingProvider`. The core `BankMappingProvider` interface has only `GetMapping` and must not change without the owner's approval.
- There is no JS toolchain in the repo, and CI runs only Go.
- Requirements: `specs/web-ui`, `specs/rest-api`.

## Goals / Non-Goals

**Goals:**
- One page, three static files, embedded in the binary: `go run ./cmd/server` (and later the Lambda) serves the UI with nothing else to build or deploy.
- Read endpoints shaped for the page but usable by any client (curl, the MCP server later).
- Caller-supplied text can't become markup or script, enforced twice: the DOM API the JS uses, and the CSP.

**Non-Goals:**
- A JS framework, bundler or JS unit tests.
- Pagination, filters, file download, live status updates, auth.

## Decisions

**1. Static files in `internal/handler/web/`, embedded with `embed.FS`, one explicit route each.**
The routes are `GET /{$}` (index.html, exact root only), `GET /app.js` and `GET /app.css`. All three go through one small helper that sets the content type, `Content-Security-Policy: default-src 'self'; frame-ancestors 'none'` and `X-Content-Type-Options: nosniff`.
- Explicit patterns rather than `http.FileServerFS`, so `routes()` stays a list `openapi_test.go` can compare one-to-one, and nothing else in the embedded directory is served by accident.
- The `{$}` matters: plain `GET /` would answer every unknown path with the page, breaking the 404 scenario and the access log's `unmatched` route.
- *Alternative:* a single HTML file with inline `<script>`/`<style>`. That needs `'unsafe-inline'` or a computed hash in the CSP; three files keep the CSP to `'self'`.

**2. Read endpoints reuse existing types.**
- `uploads.Service.List(ctx, limit int) ([]uploads.Record, error)` delegates to `recordStore.List`. `PostgresRecords.List` runs `SELECT <recordColumns> FROM uploads ORDER BY uploaded_at DESC, id LIMIT $1`, reusing `scanRecord`.
- The handler's upload dependency becomes `uploadService interface { Upload(...); List(...) }`, which `*uploads.Service` already satisfies, so `New` doesn't take an extra argument for it.
- The handler fixes the limit at 100 (`maxListedUploads`). There is no query parameter until a client needs one.
- JSON mapping is in the handler: `uploadEntry` with `*int` counts, set only when `Status == done`, so they marshal to `null` otherwise. `uploaded_at` is a `time.Time` in UTC, which `encoding/json` writes as RFC 3339.

**3. Banks come from the concrete provider through a consumer-side interface.**
- `(*FileMappingProvider).Banks() []string` returns `slices.Sorted(maps.Keys(p.mappings))`.
- The handler declares `bankLister interface { Banks() []string }` (unexported, per CLAUDE.md), and `cmd/server` passes the same provider it gives the parser.
- `BankMappingProvider` is untouched. A future runtime-detected provider can add `Banks` when it exists.
- `handler.New(up uploadService, banks bankLister, logger *slog.Logger)`.

A list response that arrives after a newer one started is ignored (a request counter), so overlapping refreshes can't show an older list.

**4. The JS builds the DOM with `createElement` + `textContent` only.**
- No `innerHTML`, `outerHTML`, `insertAdjacentHTML` or `document.write`. A Go test reads the embedded `app.js` and fails if any of them appears: a cheap guard in place of a JS test runner.
- Upload is `fetch('/uploads/csv?' + new URLSearchParams({bank, filename: file.name}), {method: 'POST', headers: {'Content-Type': 'text/csv'}, body: file})`. A `File` body is sent as raw bytes, which matches the raw-body contract ([0012](../../../docs/decisions/0012-raw-csv-upload-body.md)) and its byte-for-byte requirement.
- Times are formatted with `toLocaleString()` from the RFC 3339 value.
- After any upload response, and on the Refresh button, the page reloads `GET /uploads`.

**5. Verification of the page itself is by driving it in a browser.**
- Go tests cover everything the server does: routes, headers, JSON shapes, 404, 500, and the `innerHTML` guard.
- The page's behavior (the dropdown filled, an upload reflected in the table, error display, the markup-in-file-name case) is checked by running `cmd/server` against the compose stack and driving the page with the browser automation available to the agent. The PR records what was clicked and seen, the same exception CLAUDE.md gives thin `cmd` wiring.

**6. Logging follows 0013.**
- New routes get the access record automatically.
- A failed `GET /uploads` logs one ERROR `list uploads failed` with `method`, `route` and `error`, and answers `500 internal error`.
- Static files and `GET /banks` can't fail beyond the write, which is logged at WARN like `openAPI` does.

**7. ADR 0016 records "embedded static UI, no JS toolchain".**
That's a decision someone might reverse when the UI grows (a framework, a separate deployment). The ADR says when that would be worth it.

## Risks / Trade-offs

- [No auth on the page or the read endpoints until `X-API-Key` lands] → the server binds `localhost:8080` only, so nothing is exposed today. The API-key change has to decide how the page sends the key, which the ADR notes.
- [JS logic isn't unit-tested] → it's small (two fetches, one table, one form). The browser run is recorded in the PR, and the `innerHTML` guard protects the one property that matters most.
- [Only the newest 100 uploads are visible] → plenty for one person. A `limit`/cursor parameter is an additive change later.
- [`GET /` is now the page, so a future API index can't use the root] → acceptable. API routes keep their own paths.
- [This change is stacked on unmerged PR #124] → it branches off `feature/store-uploads-s3-postgres` and targets it, then retargets `main` once #124 merges (CLAUDE.md git workflow).

## Migration Plan

No data changes. Merge after #124, then run `go run ./cmd/server` and open `http://localhost:8080/`. Rollback: revert the PR.
