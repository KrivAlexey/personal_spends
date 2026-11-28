## Why

`docs/architecture.md` documents `GET /expenses`, `POST /expenses`, `GET /expenses/summary` and `GET /categories`, but none is built, and `storage` has no read path. The MCP tools are thin clients over these endpoints, so nothing can query spending yet (#143).

## What Changes

- `GET /expenses?from&to&category&min_amount&max_amount&limit`: stored expenses, newest first.
- `POST /expenses`: add one expense by hand, saved with `SaveExpenses` (#101).
- `GET /expenses/summary?from&to`: totals per category.
- `GET /categories`: the configured categories.
- `storage.QueryExpenses`: a DynamoDB Query on the base table (date range) or GSI1 (category and date range), with the amount bounds as a filter expression. Never a scan.
- All four routes are added to `internal/handler/openapi.yaml`.

## Capabilities

### Modified Capabilities
- `rest-api`: four new endpoints and their error behavior.

## Impact

- `internal/storage`: `ExpenseQuery`, `QueryExpenses`, `Query` on the unexported `dynamoDBAPI`.
- `internal/handler`: `New` gains an expense store and a `categories.CategoryProvider`; four handlers.
- `cmd/server`: passes the store and the category provider to `handler.New`.
- Docs: `docs/architecture.md` (status, API table, flows). No new dependency, no core interface change (`CategoryProvider` is only consumed).
- Out of scope: the MCP server, API-key auth, pagination cursors, updating or deleting expenses.
