## Purpose

Defines what the operator can rely on finding in the service's logs: which records are written at each level, the fields they carry, how records from one request are correlated, how level and format are configured, and the guarantee that bank data never reaches the logs.

## ADDED Requirements

### Requirement: Log level and format are configurable
The system SHALL write log records at or above the level named by `LOG_LEVEL` (`debug`, `info`, `warn`, `error`, case-insensitive; default `info`) and in the format named by `LOG_FORMAT` (`text` or `json`; default `text`). An unrecognized value for either variable SHALL make `logging.New` return an error naming the variable and the value. Stopping startup on that error belongs to the `cmd/*` change that wires the logger.

#### Scenario: Default configuration
- **WHEN** the service starts with neither variable set
- **THEN** INFO, WARN and ERROR records are written as text, and DEBUG records are not written

#### Scenario: Debug in JSON
- **WHEN** the service starts with `LOG_LEVEL=debug` and `LOG_FORMAT=json`
- **THEN** every record, DEBUG included, is written as one JSON object per line

#### Scenario: Invalid level
- **WHEN** `logging.New` is called with the level `verbose`
- **THEN** it returns an error that names `LOG_LEVEL` and `verbose`, and no logger

### Requirement: Every record has a timestamp, level and message
Every log record SHALL carry a timestamp, a level and a message. In JSON format these SHALL be the keys `time`, `level` and `msg`, and other fields SHALL be top-level keys with the names given in these requirements.

#### Scenario: JSON record shape
- **WHEN** any record is written with `LOG_FORMAT=json`
- **THEN** it is a JSON object containing `time`, `level` and `msg`

### Requirement: Records from one request share a request ID
The system SHALL assign each HTTP request an ID. Every record written while serving that request SHALL carry it as `request_id`, at every level and from every layer. Two concurrent requests SHALL have different IDs.

#### Scenario: Upload traced end to end
- **WHEN** an upload is served with `LOG_LEVEL=debug`
- **THEN** the access record, the processing DEBUG records and any WARN or ERROR record for that upload all carry the same `request_id`

#### Scenario: Distinct requests
- **WHEN** two uploads are served
- **THEN** their records carry different `request_id` values

### Requirement: Every request writes an access record
The system SHALL write one INFO record when it finishes serving each HTTP request, with `method`, `route`, `status` and `duration_ms`. `route` SHALL be the matched route pattern, or `unmatched` when no route matched; the record SHALL NOT include the request path, the query string or the request body.

#### Scenario: Successful upload
- **WHEN** `POST /uploads/csv?bank=sparkasse` returns `200`
- **THEN** one INFO record is written with `method=POST`, `route=POST /uploads/csv`, `status=200` and a non-negative `duration_ms`

#### Scenario: Failed upload
- **WHEN** an upload returns `500`
- **THEN** the INFO access record is still written, with `status=500`

#### Scenario: Unknown path
- **WHEN** a `GET` whose path is `/REWE-23,45` followed by 100 KB of `A` (in the path, not the body) returns `404`
- **THEN** the access record has `route=unmatched` and `status=404`, and neither `REWE` nor any part of the path appears in the log output

### Requirement: Internal errors are logged with their cause
When a request fails with `500`, the system SHALL write exactly one ERROR record carrying `method`, `route`, `bank` (when present, capped as below) and `error`, the full error chain as text. The HTTP response SHALL stay as `rest-api` specifies, with no internal details. A layer that returns an error SHALL NOT also log it, so one failure produces one ERROR record.

#### Scenario: Storage failure during upload
- **WHEN** an upload to `/uploads/csv?bank=sparkasse` fails because saving to the store fails
- **THEN** exactly one ERROR record is written with `bank=sparkasse` and an `error` that contains both the import stage (save) and the store's error text, and the response body is still `internal error`

#### Scenario: Categorizer failure during upload
- **WHEN** an upload fails because the categorizer returns an error
- **THEN** exactly one ERROR record is written, and its `error` contains the categorizer's error text

### Requirement: Client cancellations are logged as warnings
When a request fails because its own context was cancelled (the client disconnected), the system SHALL write one WARN record instead of the ERROR record, with `method`, `route`, `bank` (when present, capped) and `error`. A failure from `context.DeadlineExceeded`, or from a cancellation the request's context didn't cause, SHALL stay ERROR. The HTTP response stays as `rest-api` specifies.

#### Scenario: Client disconnects during categorization
- **WHEN** the client of `/uploads/csv?bank=sparkasse` disconnects while a categorizer batch is running, and the import returns an error wrapping `context.Canceled`
- **THEN** one WARN record is written with `bank=sparkasse` and an `error` containing `context canceled`, and no ERROR record is written

#### Scenario: Timeout stays an error
- **WHEN** an upload fails with an error wrapping `context.DeadlineExceeded`
- **THEN** one ERROR record is written, as for any other internal failure

### Requirement: Handler panics are logged and answered
When a handler panics with anything but `http.ErrAbortHandler` (a deliberate abort, see its scenario), the system SHALL write exactly one ERROR record with `method`, `route` and `stack` (stack trace, never the panic value), and still the access record, whose `status` is what the client received. A client with no response bytes yet SHALL receive `500` with the body `internal error`; if the response has started, the connection SHALL be closed so a cut-off response isn't taken as complete.

#### Scenario: Panic before the response starts
- **WHEN** the upload handler panics with the value `boom REWE` before writing anything
- **THEN** the client receives `500` with the body `internal error`, one ERROR record is written with `route=POST /uploads/csv`, the request's `request_id` and a `stack`, the access record has `status=500`, and `boom REWE` does not appear in the log output

#### Scenario: Panic after the response started
- **WHEN** a handler writes `200` and part of its body, then panics
- **THEN** one ERROR record is written, the access record is written with `status=200` (what the client received), and the connection is closed without completing the response

#### Scenario: Deliberate abort
- **WHEN** a handler panics with `http.ErrAbortHandler` before writing anything
- **THEN** no ERROR record is written and no `500` is sent, the access record has `status=0` (nothing was sent), and the connection is closed. The connection-close part holds under `net/http`; behind the Lambda adapter it is decided when `cmd/lambda` lands

### Requirement: Rejected requests are logged with the reason
When a request is rejected with `400`, the system SHALL write one WARN record with `method`, `route`, `bank` (when present) and `reason`. The `reason` SHALL be fixed text that contains no caller input. In every record, a `bank` value longer than 64 runes (Unicode characters, not bytes) SHALL be cut to its first 64 runes followed by `…`.

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

#### Scenario: Missing bank parameter
- **WHEN** an export is posted to `/uploads/csv` with no `bank` parameter
- **THEN** one WARN record is written whose `reason` says the bank parameter is missing

### Requirement: Debug records trace normal import processing
At DEBUG level, an upload SHALL write records for each processing step with these fields:
- after parsing: `bank`, `rows` (transactions parsed), `pending_skipped`
- after deduplication: `already_known`, `fresh`
- per categorizer batch: `batch` (1-based) and `batches` (total), plus the fields in the `categorizer` spec
- per saved batch: `batch`, `saved`
- at the end: `imported`, `already_known`, `pending_skipped`, `duration_ms`

None of these records SHALL be written at INFO level or above.

#### Scenario: Two-batch import
- **WHEN** an export with 60 new rows is uploaded with a batch size of 50 and `LOG_LEVEL=debug`
- **THEN** two categorizer batch records are written, with `batch=1, batches=2, size=50` and `batch=2, batches=2, size=10`, followed by a summary with `imported=60`. At the importer level this holds with any `Categorizer` that meets its contract, including the test fake

#### Scenario: Truncated Claude response is visible
- **WHEN** a categorizer batch's response stops because it reached the token limit
- **THEN** that batch's DEBUG record has `stop_reason=max_tokens`

#### Scenario: Nothing new to categorize
- **WHEN** an export whose rows are all already stored is uploaded with `LOG_LEVEL=debug`
- **THEN** the deduplication record has `fresh=0`, no categorizer batch record is written, and the summary has `imported=0`

#### Scenario: Default level stays quiet
- **WHEN** a successful upload is served with `LOG_LEVEL=info`
- **THEN** the only record written for it is the access record

### Requirement: Store retries are logged
When a store operation retries because items were left unprocessed, the system SHALL write a WARN record with `operation` (`batch_write` or `batch_get`), `attempt` and `unprocessed` (count). `attempt` counts retries: the first retry after the original call is `attempt=1`.

#### Scenario: Throttled batch write
- **WHEN** a batch write leaves 3 items unprocessed and is retried
- **THEN** before the first retry a WARN record is written with `operation=batch_write`, `attempt=1` and `unprocessed=3`

### Requirement: Bank data never reaches the logs
No log record, at any level and including the `error` field, SHALL contain the request body, a raw CSV row or cell value, a transaction's merchant, description or amount, the text of a Claude request or response, or a secret (`ANTHROPIC_API_KEY`, `API_KEY`, the `X-API-Key` header). Records MAY contain counts, durations, token usage, bank names, expense dates, category names and status codes.

#### Scenario: Debug upload of a real-looking export
- **WHEN** an export containing the merchant `REWE Markt GmbH`, the description `Kartenzahlung 12.03` and the amount `-23,45` is uploaded with `LOG_LEVEL=debug`
- **THEN** none of these appears anywhere in the log output, matched case-insensitively: `REWE`, `Kartenzahlung`, the raw amount `23,45` and its parsed form `23.45`

#### Scenario: Malformed cell value
- **WHEN** an upload fails because the amount cell `-12,3x` can't be parsed
- **THEN** the record that logs the failure names the column, and neither `12,3x` nor `12.3x` appears in the log output. The record is ERROR while `rest-api` answers parse failures with `500`; once #91 makes them `400`, the same holds for the WARN record

#### Scenario: Anthropic key in an error path
- **WHEN** an upload runs with `LOG_LEVEL=debug` through the real `categorizer.Claude`, constructed with the API key `sentinel-anthropic-key` and pointed at a local fake API server that answers `401` with an Anthropic error body
- **THEN** the fake server received `sentinel-anthropic-key` in the `X-Api-Key` header (so the key really was in play), the upload fails with one ERROR record, and `sentinel-anthropic-key` does not appear anywhere in the log output

The same rule covers the `X-API-Key` header. Nothing reads that header yet, so a test for it would pass trivially; the change that adds the auth middleware adds that scenario and its test.

#### Scenario: Store write failure
- **WHEN** writing an expense to the store fails
- **THEN** the logged error contains none of that expense's merchant or description (case-insensitive) or its amount in raw or parsed form
