## Why

Every column named in a bank's `mappings` must be in the export's header, so an export that legitimately omits an optional field's column (a single-currency account without `Waehrung`) fails as a whole (#114). Strictness stays the default: a misspelled optional key would otherwise silently empty every description.

## What Changes

- A bank mapping may list `optionalColumns`, a subset of its `mappings` keys. An export lacking an optional column imports with that field empty. Every other mapped column stays required.
- Loading fails when an optional column is not in `mappings`, maps to `Date` or `Amount`, or is also an identity column.

## Capabilities

### Modified Capabilities
- `csv-import`: "Exports must contain the mapped columns" gains the optional-column exception, and mapping validation gains the `optionalColumns` rules.

## Impact

`internal/parser` (`BankMapping`, `newLayout`, `applyDefaults`). No change to the `BankMappingProvider` method set.
