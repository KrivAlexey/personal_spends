## Decisions

- Optionality is declared per column in the mapping (`optionalColumns`), not inferred from the field: a typo in a required column must still fail, and a bank may want `Description` required.
- Listing an identity column as optional is rejected at load: a hash over a missing column would silently differ between exports.
- Only presence is optional. A present optional column is parsed like any other.
