## 1. Tests

- [x] 1.1 Failing cases in `TestBankMapping_ApplyDefaults` and a duplicate-bank test for `NewFileMappingProvider`

## 2. Validation

- [x] 2.1 `applyDefaults`: reject unknown and repeated field names, require `Date` and `Amount`
- [x] 2.2 `NewFileMappingProvider`: reject a duplicate `bankName`
- [x] 2.3 `go build ./... && go vet ./... && go test ./...` passes

## 3. Ship

- [x] 3.1 Open the PR for #99, `ci` green
- [x] 3.2 Archive this change
