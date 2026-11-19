## Why

The REST API is described only in prose (`openspec/specs/rest-api`, the table in `docs/architecture.md`). Clients such as the planned MCP server, Swagger UI or Postman can't read it from the server, and nothing stops the prose and the handler from drifting apart (#119).

## What Changes

- A hand-written OpenAPI 3.1 document, `internal/handler/openapi.yaml`, describes every route; it is embedded in the binary.
- `GET /openapi.yaml` serves it with `Content-Type: application/yaml`.
- A test fails when the server registers a route the document lacks, or the reverse, or when the upload response schema and `uploadResponse` list different fields.

## Capabilities

### New Capabilities
None.

### Modified Capabilities
- `rest-api`: adds "Serve the OpenAPI document".

## Impact

- `internal/handler/handler.go` (routes registered from a `routes()` map, new `openAPI` handler), `internal/handler/openapi.yaml`, `internal/handler/openapi_test.go`.
- No new dependencies (`go.yaml.in/yaml/v3` was already used); no core interface changes.
