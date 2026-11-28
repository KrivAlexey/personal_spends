## 1. Failing tests (one commit, red on assertions)

- [x] 1.1 `internal/storage`: `QueryExpenses` through the fake `dynamoDBAPI`: base-table and GSI1 key conditions, open-ended range, amount filter expression, newest-first, limit across pages via `LastEvaluatedKey`, no limit reads all pages, item-to-Expense mapping, API error holds no item data
- [x] 1.2 `internal/handler`: the scenarios of the delta spec for the four endpoints, and the OpenAPI tests for the new routes and schemas

## 2. Implementation

- [x] 2.1 `storage.ExpenseQuery`, `QueryExpenses`, `Query` on `dynamoDBAPI`
- [x] 2.2 Handlers, `handler.New` parameters, `internal/handler/openapi.yaml`, `cmd/server` wiring (checked by building and starting it if the local stack is available)
- [x] 2.3 `go build ./... && go vet ./... && go test ./...`

## 3. Docs and ship

- [x] 3.1 `docs/architecture.md`: Implementation Status, request flow, API table
- [x] 3.2 go-reviewer on the full diff, fix findings
- [x] 3.3 Archive this change, open the PR (`Closes #143`), `ci` green
