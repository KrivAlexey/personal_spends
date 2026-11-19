# 0016 — Embedded static web UI, no JS toolchain

**Status:** Accepted

## Context

Uploading meant `curl` and checking an upload meant `psql` (#125). The owner wants a browser page that uploads an export and lists every upload's status and counts from the `uploads` table ([0015](0015-upload-files-and-records.md)). The repo has no JS tooling and CI runs only Go. File names on the page are caller input.

## Decision

- **Three hand-written static files**, `internal/handler/web/index.html`, `app.js` and `app.css`: plain HTML, CSS and vanilla JS, embedded with `go:embed`. Every runtime that serves the API (`cmd/server`, later `cmd/lambda`) serves the page, with nothing else to build or deploy.
- **One explicit route per file**: `GET /{$}` (the root only; other unknown paths stay `404`), `GET /app.js` and `GET /app.css`. They're listed in `Handler.routes()` and `openapi.yaml` like every API route, so `TestOpenAPICoversRoutes` keeps them in step ([0014](0014-hand-written-openapi-spec.md)). Not `http.FileServerFS`, which would serve whatever lands in the directory.
- **`Content-Security-Policy: default-src 'self'; frame-ancestors 'none'`** and `X-Content-Type-Options: nosniff` on the page files. No inline script or style, so the CSP needs no `'unsafe-inline'`.
- **Caller input is only ever text:** `app.js` builds the DOM with `createElement` and `textContent`. A Go test fails if it uses `innerHTML`, `outerHTML`, `insertAdjacentHTML`, `document.write` or `eval(`.
- **The page uses public API endpoints only:** `GET /banks`, `GET /uploads` and the existing `POST /uploads/csv`, so the page and other clients see the same contract.
- **The page's behavior is checked in a real browser.** A headless Chromium run over the DevTools protocol uploads files and checks the DOM, and its screenshots go with the PR. There are no JS unit tests.

## Alternatives considered

| Option | Rejected because |
|---|---|
| React/Vite (or any bundler) SPA | A Node toolchain, a build step and a second CI job for one form and one table |
| Go `html/template` server-rendered page | Needs form-encoded or multipart upload handling, while the API takes raw bodies ([0012](0012-raw-csv-upload-body.md)). The page would be a second client of the same data with its own code path |
| A single HTML file with inline `<script>` and `<style>` | The CSP would need `'unsafe-inline'` or computed hashes |
| htmx or a similar library from a CDN | A foreign script origin under the CSP, and a dependency for two fetches |

## Consequences

- Revisit when the UI grows past a couple of views (expenses, categories, charts), when it needs to be hosted apart from the API, or when JS logic gets complex enough to need unit tests. Any of those is the point to bring in a toolchain.
- The page has no authentication, like every route today. Neither the page nor `POST /uploads/csv` checks `Origin` or `Host`. Until auth lands, any site open in the owner's browser could post a cross-origin upload to `localhost:8080`, or read `GET /uploads` through DNS rebinding. Listening on localhost only limits this to the owner's own browser, and the auth change closes it. When the `X-API-Key` middleware lands, that change has to decide how the page supplies the key (a prompt kept in `sessionStorage`, or a cookie session for the page routes).
- UI regressions aren't caught by CI. The browser run is manual and recorded per PR.
