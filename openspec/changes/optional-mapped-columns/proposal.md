## Why

Since #113, an export whose header lacks any column named in the bank's `mappings` is rejected. That includes columns of optional fields (`Merchant`, `Currency`, `Description`): a single-currency account that omits `Waehrung` fails as a whole (#114). Strictness stays the default so a misspelled key still fails loudly instead of silently emptying a field.

## What Changes

- A bank mapping may list `optionalColumns:` in `bank_mappings.yaml`. If an export's header lacks such a column, the field it maps to stays empty and the import succeeds.
- Every other mapped column stays required.
- `Date` and `Amount` columns can never be optional. Loading a mapping that lists one fails, as does listing a column that isn't in `mappings`, a duplicate or empty name, or an identity column.

## Capabilities

### Modified Capabilities
- `csv-import`: "Exports must contain the mapped columns" gains the optional-column exception.

## Impact

- `internal/parser`: `BankMapping.OptionalColumns` field (the `BankMappingProvider` method set is unchanged), validation in `applyDefaults`, `newLayout` skips missing optional columns.
- Docs: `docs/architecture.md` mapping description, if it lists mapping keys.
- No new dependencies. The committed Sparkasse mapping is unchanged.
