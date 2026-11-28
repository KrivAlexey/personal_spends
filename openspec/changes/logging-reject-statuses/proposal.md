## Why

The logging spec says the rejected-request WARN record is written for `400` only. Since #123 `Handler.reject` also answers `413` (`body too large`) and `409` (`upload in progress`) and writes the same WARN record, so the spec is narrower than the behavior (#140).

## What Changes

- The "Rejected requests are logged with the reason" requirement covers every rejection through `Handler.reject`: `400`, `409` and `413`.
- Two scenarios added: body over 10 MiB (`413`) and upload in progress (`409`).
- No code change: `TestUploadCSV_ConflictAndTooLargeRecords` already asserts both records.

## Capabilities

### Modified Capabilities
- `logging`: rejected-request WARN record widened from `400` to `400`, `409` and `413`.

## Impact

`openspec/specs/logging/spec.md` only.
