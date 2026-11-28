# Spec Delta

## MODIFIED Requirements

### Requirement: Rejected requests are logged with the reason
When a request is rejected with `400`, `409` or `413`, the system SHALL write one WARN record with `method`, `route`, `bank` (when present) and `reason`. The `reason` SHALL be fixed text that contains no caller input. In every record, a `bank` value longer than 64 runes (Unicode characters, not bytes) SHALL be cut to its first 64 runes followed by `…`.

#### Scenario: Unknown bank
- **WHEN** an export is posted to `/uploads/csv?bank=nobank`
- **THEN** one WARN record is written with `bank=nobank` and a `reason` saying the bank mapping was not found, and the `reason` does not contain `nobank`

#### Scenario: Oversized bank parameter
- **WHEN** an export is posted with a `bank` parameter of 10,000 characters
- **THEN** the WARN record's `bank` is the first 64 runes followed by `…`, and no record contains the full value

#### Scenario: Bank parameter that imitates a record
- **WHEN** an export is posted with a `bank` parameter of `x\nlevel=ERROR msg=forged` (with a real newline), in either `LOG_FORMAT`
- **THEN** exactly one WARN record is written for the rejection and the output has no line starting with `level=ERROR`; the value is escaped inside the `bank` field

#### Scenario: Oversized multi-byte bank parameter
- **WHEN** an export is posted with a `bank` parameter of `ä` repeated 100 times
- **THEN** the WARN record's `bank` is `ä` repeated 64 times followed by `…`, and contains no U+FFFD replacement character

#### Scenario: Body too large
- **WHEN** a body over 10 MiB is posted to `/uploads/csv?bank=sparkasse`
- **THEN** the response is `413` and one WARN record is written with `bank=sparkasse` and `reason` `body too large`, and no ERROR record is written

#### Scenario: Upload in progress
- **WHEN** an export is posted while the same file is already being imported
- **THEN** the response is `409` and one WARN record is written with `reason` `upload in progress`, and no ERROR record is written

#### Scenario: Missing bank parameter
- **WHEN** an export is posted to `/uploads/csv` with no `bank` parameter
- **THEN** one WARN record is written whose `reason` says the bank parameter is missing
