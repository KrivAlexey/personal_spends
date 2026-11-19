# Tasks

## 1. Setup

- [x] 1.1 Open the GitHub issue; branch `feature/uploads-web-ui` off `feature/store-uploads-s3-postgres` (PR #124, unmerged) and plan the PR against it; verify `git log` shows #124's commits as the base

## 2. Bank list (`internal/parser`)

- [x] 2.1 Failing table-driven test in `mapping_test.go` for `(*FileMappingProvider).Banks()`: the committed mappings give `["sparkasse"]`, a temp file with `zbank` and `abank` gives `["abank", "zbank"]` (sorted), and the result is a fresh slice the caller can modify without affecting the next call; add a stub returning nil so it fails on assertions; commit the tests alone
- [x] 2.2 Implement `Banks()` with `slices.Sorted(maps.Keys(...))`; verify 2.1 passes and `BankMappingProvider` is unchanged; commit

## 3. Upload list (`internal/uploads`)

- [x] 3.1 Failing tests:
  - `Service.List` passes the limit through and returns the records' errors wrapped (fake `recordStore` gains `List`)
  - `PostgresRecords.List` integration test (skipped without `TEST_DATABASE_URL`): newest first by `uploaded_at`, `limit` honored (3 records, limit 2 gives the 2 newest), empty table gives an empty non-nil slice, a done record carries counts and a failed one doesn't

  Stub `List` methods so the tests fail on assertions; run the integration test against compose Postgres; commit the tests alone
- [x] 3.2 Implement `recordStore.List`, `PostgresRecords.List` (`ORDER BY uploaded_at DESC, id LIMIT $1`, reusing `scanRecord`) and `Service.List`; verify 3.1 passes with and without `TEST_DATABASE_URL`; commit

## 4. REST read endpoints and page routes (`internal/handler`)

- [x] 4.1 Failing handler tests (the fake uploader gains `List`, plus a fake `bankLister`):
  - `GET /uploads` returns `{"uploads": [...]}` newest first, with every field, `null` counts unless done, `uploaded_at` RFC 3339 UTC, `[]` when empty, limit 100 passed to `List`
  - a `List` error gives `500` without the error text, plus one ERROR `list uploads failed`
  - `GET /banks` returns `{"banks": ["abank", "sparkasse"]}`
  - `GET /` returns `text/html; charset=utf-8` and references `/app.js` and `/app.css`; `GET /app.js` returns `text/javascript; charset=utf-8`, `GET /app.css` returns `text/css; charset=utf-8`
  - all three page responses carry `Content-Security-Policy: default-src 'self'; frame-ancestors 'none'` and `X-Content-Type-Options: nosniff`
  - `GET /nothing-here` is `404` and logged as route `unmatched`
  - the embedded `app.js` contains none of `innerHTML`, `outerHTML`, `insertAdjacentHTML`, `document.write`
  - `openapi.yaml` documents the five new routes with matching content types (`TestOpenAPICoversRoutes` plus a schema test for the list entry and banks bodies)

  Stub the new routes and `New` signature so it compiles; confirm assertion failures; commit the tests alone
- [x] 4.2 Implement: `New(up uploadService, banks bankLister, logger)`, `listUploads` / `uploadEntry`, `listBanks`, the embedded `web/` files served through one header-setting helper, and the `openapi.yaml` entries; write `web/index.html`, `web/app.css` and `web/app.js` (bank dropdown, file picker, Upload, result/error area, uploads table, Refresh, "no uploads yet"; DOM built with `createElement`/`textContent` only); verify 4.1 and all existing handler tests pass; commit

## 5. Wiring, check in the browser, docs

- [x] 5.1 Wire `cmd/server`: pass the mapping provider as `bankLister` and `uploads.Service` as `uploadService`; verify `go build ./...` (thin-wiring exception, checked in 5.2)
- [x] 5.2 Run `cmd/server` against the compose stack (`CATEGORIZER=fake`) and drive `http://localhost:8080/` in the browser. Check:
  - the dropdown lists `sparkasse`
  - "no uploads yet" on an empty table
  - uploading `samples/sparkasse_fake.csv` shows its `upload_id` and counts and adds a `done` row
  - uploading it again shows the same `upload_id`
  - a file over 10 MiB shows the 413
  - a file named `<img src=x onerror=alert(1)>.csv` is listed as literal text, with no image element in the DOM
  - the browser console shows no CSP violations

  Record what was seen for the PR
- [x] 5.3 ADR `0016-embedded-static-web-ui.md`: embedded HTML/CSS/vanilla JS, no toolchain, explicit routes, CSP, when to revisit (framework, separate hosting, auth via `X-API-Key`); verify it is linked from `docs/architecture.md`
- [x] 5.4 Update `docs/architecture.md`: Implementation Status (handler, uploads, parser), API table (`GET /uploads`, `GET /banks`, page routes), a short Web UI section, and the system diagram's browser entry. Update the README ("open http://localhost:8080/" in Run locally and Offline). Verify each section against the code
- [x] 5.5 Run `go build ./... && go vet ./... && go test ./...`, the integration tests with `TEST_DATABASE_URL`/`TEST_S3_ENDPOINT`, and `openspec validate add-uploads-web-ui --strict`; verify all pass

## 6. Review

- [ ] 6.1 Run the go-reviewer agent with `git diff feature/store-uploads-s3-postgres...HEAD` (or `main...HEAD` if #124 has merged), the matching `git log --oneline` and the build/vet/test output; fix its bug and risk findings (stop and ask if a fix needs a core interface change, a new dependency or a skipped test); verify `go test ./...` passes again

## Workflow follow-up

- Open the PR against `feature/store-uploads-s3-postgres` (retarget to `main` once #124 merges), fill the template (Not verified: the JS has no unit tests; browser checks are manual), and wait for `ci` with `gh pr checks <PR> --watch`
- Archive this change after `store-uploads-s3-postgres` is archived (its `upload-tracking` spec is referenced here), then verify `openspec/specs/web-ui` and `openspec/specs/rest-api` hold the merged requirements
