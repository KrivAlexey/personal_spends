# Proposal

## Why

Uploads now leave a record in PostgreSQL (change `store-uploads-s3-postgres`, PR #124), but the only way to upload is `curl` and the only way to see what happened is `psql`. A small web page served by the same server lets the owner pick a bank, upload an export, and see every upload's status and counts in a browser. It also gives the REST API its first read endpoints.

## What Changes

- New page at `GET /`, served by the Go server from embedded files (`index.html`, `app.js`, `app.css`). Plain HTML, CSS and vanilla JS: no Node toolchain, no new dependency.
  - **Upload form:** a bank dropdown, a file picker and an Upload button. It posts the file's raw bytes to the existing `POST /uploads/csv?bank=…&filename=…` and shows the result (`upload_id`, counts) or the error.
  - **Uploads table**, newest first: file name, bank, upload time, status and, for `done` uploads, imported / already known / pending-skipped counts. It refreshes after each upload and on a Refresh button.
- New `GET /uploads`: JSON list of the 100 most recent upload records.
- New `GET /banks`: JSON list of the configured bank mapping names, for the dropdown.
- The page's responses carry a strict `Content-Security-Policy` (`default-src 'self'`), and caller-supplied text (file names) is always rendered as text, never as HTML.
- `openapi.yaml` documents the five new routes (`/`, `/app.js`, `/app.css`, `/uploads`, `/banks`).
- Out of scope:
  - downloading stored files from S3
  - pagination or filters beyond the newest 100
  - auto-refresh or polling while an upload is `processing`
  - authentication (the `X-API-Key` middleware isn't built yet; the page gets the same auth as every route when it lands)
  - expenses or categories views

## Capabilities

### New Capabilities
- `web-ui`: the browser page: what it shows about uploads, how uploading from it works, and how it treats caller-supplied text.

### Modified Capabilities
- `rest-api`: adds `GET /uploads` and `GET /banks`. The OpenAPI requirement already covers documenting every served route, so its text doesn't change. The page routes are added there too.

## Impact

- **Depends on** PR #124 (`uploads` table, `uploads.Service`, `upload_id`). This change is branched off it and targets it until it merges.
- **Code:**
  - `internal/handler`: new routes, embedded `web/` assets, a narrow `bankLister` interface, and the upload interface gaining `List`
  - `internal/uploads`: `Service.List` and `PostgresRecords.List`
  - `internal/parser`: `FileMappingProvider.Banks()` on the concrete type only. The core `BankMappingProvider` interface is unchanged.
  - `cmd/server`: wiring
- **Docs:** `docs/architecture.md` (status, API table, a "Web UI" section) and README (open `localhost:8080`). No ADR is needed unless the embedded-assets choice should be recorded; the design proposes one short ADR.
- No new dependencies, no AWS resources, no Terraform.
