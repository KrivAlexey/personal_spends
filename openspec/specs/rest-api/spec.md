# rest-api Specification

## Purpose
Defines the HTTP contract of the REST API that clients (curl, the MCP server) rely on: request shapes, response bodies and status codes.

## Requirements

### Requirement: Upload a bank export
The system SHALL accept `POST /uploads/csv?bank=<bankName>` with the bank export as the raw request body and an optional `filename` parameter, import it as specified by `csv-import` and `upload-tracking`, and respond `200` with a JSON body (`Content-Type: application/json`) holding the string field `upload_id` and the integer fields `imported`, `already_known` and `pending_skipped`. All four fields SHALL be present, including when a count is 0.

#### Scenario: Mixed export
- **WHEN** a Sparkasse export with 2 pending rows, 3 already-stored rows and 5 new rows is posted to `/uploads/csv?bank=sparkasse&filename=statement.csv`
- **THEN** the response is `200` with a non-empty `upload_id` and `"imported": 5, "already_known": 3, "pending_skipped": 2`

#### Scenario: Nothing new
- **WHEN** an export whose only row is already stored, and whose bytes were never uploaded before, is posted
- **THEN** the response is `200` with a non-empty `upload_id` and `"imported": 0, "already_known": 1, "pending_skipped": 0`

### Requirement: Upload body is passed through unchanged
The system SHALL hand the request body to the import byte for byte, without re-encoding it, so the bank mapping's declared encoding applies to the bytes the client sent.

#### Scenario: ISO-8859-1 export
- **WHEN** a Sparkasse export containing the ISO-8859-1 bytes for `Grundpreis für Kontoführung` is posted
- **THEN** the import receives exactly those bytes, and the stored description is `Grundpreis für Kontoführung`

### Requirement: Invalid uploads are rejected
The system SHALL respond `400` and import nothing when the `bank` parameter is missing or empty, when the body is empty or cannot be read, or when no bank mapping exists for `bank`, unless the body repeats a finished upload, which is answered as specified by "Repeated uploads return the original result".

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

#### Scenario: Unknown bank on a finished file
- **WHEN** an export was uploaded and finished under `bank=sparkasse`, and the identical bytes are posted to `/uploads/csv?bank=nobank`
- **THEN** the response is `200` with the first upload's `upload_id` and counts, and nothing is imported

### Requirement: Import failures do not leak internals
The system SHALL respond `500` when an upload fails for any reason not covered by "Invalid uploads are rejected", and the response body SHALL NOT contain the internal error message.

#### Scenario: Storage failure
- **WHEN** storing the categorized expenses fails with an error naming the DynamoDB table
- **THEN** the response is `500` and its body does not contain the table name

### Requirement: Serve the OpenAPI document
The system SHALL respond to `GET /openapi.yaml` with `200`, `Content-Type: application/yaml` and an OpenAPI 3.1 document that describes every route the server serves, including `GET /openapi.yaml` itself, and no route it does not serve.

#### Scenario: Fetch the document
- **WHEN** a client sends `GET /openapi.yaml`
- **THEN** the response is `200` with `Content-Type: application/yaml` and an OpenAPI 3.1 document listing `POST /uploads/csv` and `GET /openapi.yaml`

#### Scenario: Wrong method
- **WHEN** a client sends `POST /openapi.yaml`
- **THEN** the response is `405`

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

### Requirement: List recent uploads
The system SHALL respond to `GET /uploads` with `200` and a JSON body (`Content-Type: application/json`) `{"uploads": [...]}` holding the 100 most recently received upload records, newest first by the time each file was first received, as defined by `upload-tracking`. A retried upload keeps the position of its first receipt. The list SHALL be `[]`, not `null`, when there are none.

#### Scenario: Two uploads
- **WHEN** `a.csv` was received, then `b.csv`, and a client sends `GET /uploads`
- **THEN** the response is `200` and `uploads` lists `b.csv` first, then `a.csv`

#### Scenario: No uploads
- **WHEN** no upload was ever recorded and a client sends `GET /uploads`
- **THEN** the response is `200` with `{"uploads": []}`

#### Scenario: Retried upload keeps its place
- **WHEN** `a.csv` failed, then `b.csv` was received, then `a.csv` was posted again and finished
- **THEN** `uploads` lists `b.csv` first, then `a.csv` with status `done`

#### Scenario: More than 100 uploads
- **WHEN** 101 uploads were recorded and a client sends `GET /uploads`
- **THEN** `uploads` holds 100 entries and omits the oldest one

### Requirement: Upload list entries
Each entry of `GET /uploads` SHALL hold `upload_id` (string), `filename` (string, empty when none was given), `bank` (string), `uploaded_at` (RFC 3339 timestamp in UTC), `status` (`processing`, `done` or `failed`), and `imported`, `already_known` and `pending_skipped`, which are integers when `status` is `done` and `null` otherwise. All fields SHALL be present.

#### Scenario: Done upload
- **WHEN** `statement.csv` was uploaded under `sparkasse` on 2026-11-19 at 17:01:03 UTC and finished with counts 5, 3 and 2
- **THEN** its entry is `{"upload_id": "<id>", "filename": "statement.csv", "bank": "sparkasse", "uploaded_at": "2026-11-19T17:01:03Z", "status": "done", "imported": 5, "already_known": 3, "pending_skipped": 2}`, with `uploaded_at` possibly carrying fractional seconds

#### Scenario: Failed upload
- **WHEN** an upload failed
- **THEN** its entry has `"status": "failed"` and `null` for `imported`, `already_known` and `pending_skipped`

### Requirement: Upload list failures do not leak internals
The system SHALL respond `500` when the upload records can't be read, and the response body SHALL NOT contain the internal error message.

#### Scenario: Database down
- **WHEN** reading the upload records fails with an error naming the database host
- **THEN** the response to `GET /uploads` is `500` and its body does not contain the host name

### Requirement: List configured banks
The system SHALL respond to `GET /banks` with `200` and a JSON body `{"banks": [...]}` holding the name of every configured bank mapping, sorted ascending: exactly the values `POST /uploads/csv` accepts as `bank`. With no mappings the list SHALL be `[]`, not `null`.

#### Scenario: One mapping
- **WHEN** the bank mappings define only `sparkasse` and a client sends `GET /banks`
- **THEN** the response is `200` with `{"banks": ["sparkasse"]}`
