## 1. Tests and decision

- [x] 1.1 ADR 0012 and `docs/architecture.md` upload flow + API table
- [x] 1.2 Failing tests in `internal/handler/handler_test.go` covering every `rest-api` scenario (the ISO-8859-1 scenario's stored-text half is already covered by the parser's `csv-import` tests)

## 2. Handler

- [x] 2.1 Implement `uploadCSV`: read and validate `bank`, reject an empty body, call `Import` with the request context and body
- [x] 2.2 Map `parser.ErrBankMappingNotFound` (via `errors.Is`) to `400`, any other error to a generic `500`
- [x] 2.3 Encode the result as JSON with `imported`, `already_known`, `pending_skipped`
- [x] 2.4 `go build ./... && go vet ./... && go test ./...` passes

## 3. Ship

- [x] 3.1 Update `docs/architecture.md` Implementation Status with an `internal/handler` row
- [ ] 3.2 Open the PR for #77, `ci` green
- [ ] 3.3 Archive this change once merged
