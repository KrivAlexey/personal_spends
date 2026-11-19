# 0014 — Hand-written OpenAPI spec, embedded and served, kept honest by a test

**Status:** Accepted

## Context

The REST API was documented only in prose (#119). Clients — the planned MCP server, Swagger UI, Postman — need a machine-readable description, and it must not drift from the handler. The API is one endpoint today and grows one endpoint at a time.

## Decision

- **Hand-written OpenAPI 3.1** in `internal/handler/openapi.yaml`, next to the code it describes.
- **Embedded with `go:embed` and served at `GET /openapi.yaml`** (`application/yaml`), so the running binary always serves the document it was built with. It gets the same auth as every other route once the `X-API-Key` middleware lands.
- **A test, not a generator, prevents drift.** `Handler.routes()` is the one list of ServeMux patterns; `openapi_test.go` fails when it and the document's operations differ, or when the upload response schema and `uploadResponse`'s JSON fields differ.

## Alternatives considered

| Option | Rejected because |
|---|---|
| Generate server code from the spec (oapi-codegen) | A new dependency and generated routing for one endpoint; the handler's error handling is hand-tuned (ADR 0012, 0013) |
| Generate the spec from code annotations (swaggo) | A new dependency and comment DSL; OpenAPI 2.0 only |
| Spec in `docs/`, not served | Clients can't fetch it from the server they talk to |
| Swagger UI bundled | Pulls in JS assets; any viewer can load `/openapi.yaml` |

## Consequences

- Every new route needs an `openapi.yaml` entry in the same PR, or `TestOpenAPICoversRoutes` fails.
- Response schemas other than the upload result are checked by review only, until a test like `TestOpenAPIUploadResponseSchema` is added for them.
- Behavior stays specified in `openspec/specs/rest-api`; the OpenAPI document is the wire-format description of it.
