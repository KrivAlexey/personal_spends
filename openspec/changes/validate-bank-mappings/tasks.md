## 1. Tests

- [ ] 1.1 Failing cases in `TestBankMapping_ApplyDefaults` and a duplicate-bank test for `NewFileMappingProvider`

## 2. Validation

- [ ] 2.1 `applyDefaults`: reject unknown and repeated field names, require `Date` and `Amount`
- [ ] 2.2 `NewFileMappingProvider`: reject a duplicate `bankName`
- [ ] 2.3 `go build ./... && go vet ./... && go test ./...` passes

## 3. Ship

- [ ] 3.1 Open the PR for #99, `ci` green
- [ ] 3.2 Archive this change
