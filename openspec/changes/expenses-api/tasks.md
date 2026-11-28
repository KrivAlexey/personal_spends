## 1. Failing tests (one commit, red on assertions)

- [ ] 1.1 `internal/storage`: `QueryExpenses` through the fake `dynamoDBAPI`: base-table and GSI1 key conditions, open-ended range, amount filter expression, newest-first, limit across pages via `LastEvaluatedKey`, no limit reads all pages, item-to-Expense mapping, API error holds no item data
- [ ] 1.2 `internal/handler`: the scenarios of the delta spec for the four endpoints, and the OpenAPI tests for the new routes and schemas

## 2. Implementation

- [ ] 2.1 `storage.ExpenseQuery`, `QueryExpenses`, `Query` on `dynamoDBAPI`
- [ ] 2.2 Handlers, `handler.New` parameters, `internal/handler/openapi.yaml`, `cmd/server` wiring (checked by building and starting it if the local stack is available)
- [ ] 2.3 `go build ./... && go vet ./... && go test ./...`

## 3. Docs and ship

- [ ] 3.1 `docs/architecture.md`: Implementation Status, request flow, API table
- [ ] 3.2 go-reviewer on the full diff, fix findings
- [ ] 3.3 Archive this change, open the PR (`Closes #143`), `ci` green
