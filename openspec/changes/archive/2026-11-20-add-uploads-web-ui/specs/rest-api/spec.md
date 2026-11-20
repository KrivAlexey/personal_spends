# Spec Delta

## ADDED Requirements

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
