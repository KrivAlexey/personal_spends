## 1. Failing tests

- [x] 1.1 `internal/parser`: export missing only an optional column imports with that field empty; missing a non-optional mapped column still fails; mapping validation rejects optional Date/Amount columns, unknown, duplicate, empty and identity columns

## 2. Implementation

- [x] 2.1 `BankMapping.OptionalColumns`, validation in `applyDefaults`, `newLayout` tolerates missing optional columns

## 3. Docs and review

- [x] 3.1 Check `docs/architecture.md`, README, CLAUDE.md against the change
- [x] 3.2 Run the go-reviewer agent on the full diff and fix its bug and risk findings
