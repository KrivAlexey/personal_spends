## Context

`newLayout` rejects any header lacking a mapped column. That stays the default.

## Decisions

`optionalColumns` is an explicit list, not "any non-Date/Amount column is optional", so a typo in a mapping key still fails loudly unless the owner opted that column in. `newLayout` skips the missing-column check for listed columns; `parseRows` already leaves a field empty when no column fills it. An optional column that is also an identity column is rejected on load, since the identity check would fail the export anyway.
