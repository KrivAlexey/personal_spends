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

### Requirement: Expense fields
Every expense in a response SHALL have the fields `id`, `date` (`YYYY-MM-DD`), `amount` (number), `currency`, `merchant`, `raw_description`, `category`, `confidence`, `source` and `created_at` (RFC 3339), all always present.

#### Scenario: Stored expense
- **WHEN** an expense with every field set is stored and listed
- **THEN** the listed element has all ten fields

### Requirement: Query expenses
The system SHALL answer `GET /expenses` with `200` and a JSON body `{"expenses": [...]}`, newest booking date first, each element in the shape of "Expense fields". The optional parameters narrow the result: `from` and `to` (`YYYY-MM-DD`, both inclusive) bound the booking date, `category` keeps one category, `min_amount` and `max_amount` (inclusive, signed decimals) bound the amount, and `limit` caps the number of expenses (default 100, at most 500). With no match the list SHALL be empty.

#### Scenario: Date range
- **WHEN** expenses dated 2026-03-31, 2026-04-01, 2026-04-30 and 2026-05-01 are stored and `GET /expenses?from=2026-04-01&to=2026-04-30` is requested
- **THEN** the response lists the 2026-04-30 expense, then the 2026-04-01 one

#### Scenario: Category and amount bounds
- **WHEN** `GET /expenses?category=groceries&min_amount=-50&max_amount=-10` is requested and groceries expenses of -5, -20 and -60 are stored
- **THEN** only the -20 expense is listed

#### Scenario: Limit
- **WHEN** 3 expenses match and `limit=2` is requested
- **THEN** the 2 newest are listed

#### Scenario: No match
- **WHEN** nothing is stored
- **THEN** the response is `200` with `{"expenses": []}`

### Requirement: Invalid expense queries are rejected
The system SHALL respond `400` when `from` or `to` is not a date, `from` is after `to`, `min_amount` or `max_amount` is not a number, `min_amount` is above `max_amount`, or `limit` is not an integer from 1 to 500. Nothing SHALL be read from storage.

#### Scenario: Bad date
- **WHEN** `GET /expenses?from=yesterday` is requested
- **THEN** the response is `400`

#### Scenario: Inverted range
- **WHEN** `GET /expenses?from=2026-05-01&to=2026-04-01` is requested
- **THEN** the response is `400`

#### Scenario: Limit out of range
- **WHEN** `GET /expenses?limit=501` is requested
- **THEN** the response is `400`

### Requirement: Add an expense
The system SHALL accept `POST /expenses` with a JSON body holding `date` (`YYYY-MM-DD`), `amount` (number, not 0), `merchant` (non-empty), `category` and an optional `raw_description`, store it, and respond `201` with the stored expense in the shape of "Expense fields". The category MUST be a configured one. The expense SHALL then appear in `GET /expenses`.

#### Scenario: Valid expense
- **WHEN** `{"date":"2026-04-02","amount":-12.5,"merchant":"Bakery","category":"groceries"}` is posted
- **THEN** the response is `201` with `source` `manual`, `currency` `EUR`, `confidence` `1` and a non-empty `id`, and the expense is stored

#### Scenario: Same body twice
- **WHEN** the same body is posted twice
- **THEN** both responses are `201` with different ids

### Requirement: Server-set fields of an added expense
The system SHALL set `currency` to `EUR`, `confidence` to `1`, `source` to `manual`, `created_at` to the current time and `id` to a new unique value on an expense added with `POST /expenses`.

#### Scenario: Fields set
- **WHEN** a valid expense is posted
- **THEN** the stored expense has `currency` `EUR`, `confidence` `1`, `source` `manual`, a `created_at` within a minute of now and a non-empty `id`

### Requirement: Invalid expenses are rejected
The system SHALL respond `400` and store nothing when the body is not a JSON object, has an unknown field, lacks `date`, `amount`, `merchant` or `category`, has an unparseable date, a zero amount or an empty merchant, or names a category that is not configured. A body over 64 KiB SHALL be answered `413`.

#### Scenario: Unknown category
- **WHEN** a body with `"category":"nonsense"` is posted
- **THEN** the response is `400` and nothing is stored

#### Scenario: Missing merchant
- **WHEN** a body without `merchant` is posted
- **THEN** the response is `400` and nothing is stored

#### Scenario: Not JSON
- **WHEN** `hello` is posted
- **THEN** the response is `400` and nothing is stored

### Requirement: Spending summary
The system SHALL answer `GET /expenses/summary` with `200` and a JSON body `{"categories": [...], "total": <number>, "count": <integer>}` where each element has `category`, `total` (sum of amounts, rounded to cents) and `count`, sorted by category name. The top-level `total` and `count` cover all expenses in the range. The optional `from` and `to` bound the booking date as in "Query expenses" and are rejected with `400` under the same rules.

#### Scenario: Two categories
- **WHEN** expenses of -10.10 and -20.20 in `groceries` and -5 in `fuel` are stored and `GET /expenses/summary` is requested
- **THEN** the response lists `fuel` with total -5 and count 1, then `groceries` with total -30.3 and count 2, and `total` is -35.3 and `count` 3

#### Scenario: Empty range
- **WHEN** nothing is stored in the range
- **THEN** the response is `200` with `{"categories": [], "total": 0, "count": 0}`

### Requirement: List categories
The system SHALL answer `GET /categories` with `200` and a JSON body `{"categories": [{"name": ..., "description": ...}]}` listing the configured categories.

#### Scenario: Configured categories
- **WHEN** the categories `groceries` and `fuel` are configured and `GET /categories` is requested
- **THEN** the response lists both with their descriptions

### Requirement: Expense endpoint failures do not leak internals
The system SHALL respond `500` when reading or writing expenses, or listing categories, fails, and the response body SHALL NOT contain the internal error message.

#### Scenario: Storage failure
- **WHEN** reading expenses fails with an error naming the DynamoDB table
- **THEN** the response is `500` and its body does not contain the table name
