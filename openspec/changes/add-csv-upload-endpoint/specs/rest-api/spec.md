## Purpose

Defines the HTTP contract of the REST API that clients (curl, the MCP server) rely on: request shapes, response bodies and status codes.

## ADDED Requirements

### Requirement: Upload a bank export
The system SHALL accept `POST /uploads/csv?bank=<bankName>` with the bank export as the raw request body, import it as specified by `csv-import`, and respond `200` with a JSON body (`Content-Type: application/json`) holding the integer fields `imported`, `already_known` and `pending_skipped`. All three fields SHALL be present, including when their value is 0.

#### Scenario: Mixed export
- **WHEN** a Sparkasse export with 2 pending rows, 3 already-stored rows and 5 new rows is posted to `/uploads/csv?bank=sparkasse`
- **THEN** the response is `200` with `{"imported": 5, "already_known": 3, "pending_skipped": 2}`

#### Scenario: Nothing new
- **WHEN** an export whose only row is already stored is posted
- **THEN** the response is `200` with `{"imported": 0, "already_known": 1, "pending_skipped": 0}`

### Requirement: Upload body is passed through unchanged
The system SHALL hand the request body to the import byte for byte, without re-encoding it, so the bank mapping's declared encoding applies to the bytes the client sent.

#### Scenario: ISO-8859-1 export
- **WHEN** a Sparkasse export containing the ISO-8859-1 bytes for `Grundpreis für Kontoführung` is posted
- **THEN** the import receives exactly those bytes, and the stored description is `Grundpreis für Kontoführung`

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

#### Scenario: Unknown bank
- **WHEN** an export is posted to `/uploads/csv?bank=nobank` and no mapping named `nobank` exists
- **THEN** the response is `400` and nothing is stored

### Requirement: Import failures do not leak internals
The system SHALL respond `500` when an upload fails for any reason not covered by "Invalid uploads are rejected", and the response body SHALL NOT contain the internal error message.

#### Scenario: Storage failure
- **WHEN** storing the categorized expenses fails with an error naming the DynamoDB table
- **THEN** the response is `500` and its body does not contain the table name
