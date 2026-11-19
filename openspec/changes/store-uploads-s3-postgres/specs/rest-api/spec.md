# Spec Delta

## MODIFIED Requirements

### Requirement: Upload a bank export
The system SHALL accept `POST /uploads/csv?bank=<bankName>` with the bank export as the raw request body and an optional `filename` parameter, import it as specified by `csv-import` and `upload-tracking`, and respond `200` with a JSON body (`Content-Type: application/json`) holding the string field `upload_id` and the integer fields `imported`, `already_known` and `pending_skipped`. All four fields SHALL be present, including when a count is 0.

#### Scenario: Mixed export
- **WHEN** a Sparkasse export with 2 pending rows, 3 already-stored rows and 5 new rows is posted to `/uploads/csv?bank=sparkasse&filename=statement.csv`
- **THEN** the response is `200` with a non-empty `upload_id` and `"imported": 5, "already_known": 3, "pending_skipped": 2`

#### Scenario: Nothing new
- **WHEN** an export whose only row is already stored, and whose bytes were never uploaded before, is posted
- **THEN** the response is `200` with a non-empty `upload_id` and `"imported": 0, "already_known": 1, "pending_skipped": 0`

### Requirement: Invalid uploads are rejected
The system SHALL respond `400` and import nothing when the `bank` parameter is missing or empty, when the body is empty or cannot be read, or when no bank mapping exists for `bank`.

#### Scenario: Missing bank parameter
- **WHEN** an export is posted to `/uploads/csv` with no `bank` parameter
- **THEN** the response is `400` and nothing is imported

#### Scenario: Empty bank parameter
- **WHEN** an export is posted to `/uploads/csv?bank=`
- **THEN** the response is `400` and nothing is imported

#### Scenario: Empty body
- **WHEN** an empty body is posted to `/uploads/csv?bank=sparkasse`
- **THEN** the response is `400` and nothing is imported

#### Scenario: Unreadable body
- **WHEN** the client's connection drops before the body arrives
- **THEN** the response is `400` and nothing is imported

#### Scenario: Body cut off mid-upload
- **WHEN** the client's connection drops after part of the export has arrived
- **THEN** the response is `400` and no expense is stored

#### Scenario: Unknown bank
- **WHEN** an export is posted to `/uploads/csv?bank=nobank` and no mapping named `nobank` exists
- **THEN** the response is `400` and no expense is stored

## ADDED Requirements

### Requirement: Repeated uploads return the original result
The system SHALL answer a `POST /uploads/csv` whose body is byte-identical to an earlier upload that finished with `200` and that earlier upload's `upload_id` and counts, whatever `bank` and `filename` the repeat sends.

#### Scenario: Same file posted twice
- **WHEN** an export is posted and answered `{"upload_id": "u1", "imported": 5, "already_known": 0, "pending_skipped": 2}`, then the identical bytes are posted again
- **THEN** the second response is `200` with `{"upload_id": "u1", "imported": 5, "already_known": 0, "pending_skipped": 2}`

### Requirement: Upload already in progress is refused
The system SHALL respond `409` and import nothing when the body is byte-identical to an upload that is still being processed, as defined by `upload-tracking`.

#### Scenario: Double submit
- **WHEN** a client posts the same export twice and the second request arrives while the first is importing
- **THEN** the second response is `409` and the first completes normally

### Requirement: Oversized uploads are rejected
The system SHALL respond `413` and neither keep nor import the file when the request body is larger than 10 MiB (10,485,760 bytes).

#### Scenario: Body over the limit
- **WHEN** a body of 10,485,761 bytes is posted to `/uploads/csv?bank=sparkasse`
- **THEN** the response is `413`, no file is kept and nothing is imported

#### Scenario: Body at the limit
- **WHEN** a valid export of exactly 10,485,760 bytes is posted
- **THEN** it is not rejected for its size
