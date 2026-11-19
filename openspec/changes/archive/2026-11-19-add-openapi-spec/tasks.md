## 1. Tests

- [x] 1.1 Failing tests: endpoint serves the embedded document, spec routes equal registered routes, upload response schema matches `uploadResponse`

## 2. Implementation

- [x] 2.1 Write `openapi.yaml` for `POST /uploads/csv` and `GET /openapi.yaml`
- [x] 2.2 Serve it at `GET /openapi.yaml`
- [x] 2.3 `go build ./... && go vet ./... && go test ./...` passes

## 3. Ship

- [x] 3.1 Open the PR for #119, `ci` green
- [x] 3.2 Archive this change
