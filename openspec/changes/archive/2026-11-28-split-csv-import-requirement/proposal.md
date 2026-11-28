## Why

Since #159 (#114) the csv-import requirement "Exports must contain the mapped columns" is over 500 characters, so `openspec validate --all --strict` fails on `main` (#162).

## What Changes

- "Exports must contain the mapped columns" keeps only the header rule and its `optionalColumns` exception.
- The load-time checks on `optionalColumns` move to a new requirement, "Optional columns are validated on load", with an "Optional column not in mappings" scenario. The Date/Amount scenario stays with the header requirement, whose text keeps one sentence for it (OpenSpec does not let a MODIFIED block drop a scenario).
- No behavior change and no code change: the same rules, split in two.

## Capabilities

### Modified Capabilities
- `csv-import`: one requirement split in two, same behavior.

## Impact

`openspec/specs/csv-import/spec.md` only.
