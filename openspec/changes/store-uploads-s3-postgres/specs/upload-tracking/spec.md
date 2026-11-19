# Spec Delta

## Purpose

Keeps every uploaded bank export and a record of what happened to it, so an upload can be traced, re-processed after a failure, and recognized when the same file is posted again.

## ADDED Requirements

### Requirement: Uploaded files are kept unchanged
The system SHALL store the body of every upload that passes request validation, byte for byte, before importing it, under a key derived from the SHA-256 hash of its bytes. Uploads rejected before import (missing `bank`, empty or unreadable body, body too large) SHALL NOT be stored.

#### Scenario: File kept
- **WHEN** a Sparkasse export is posted to `/uploads/csv?bank=sparkasse`
- **THEN** the stored file's bytes equal the request body, and it can be found by the hex SHA-256 of those bytes

#### Scenario: Rejected upload not kept
- **WHEN** an empty body is posted to `/uploads/csv?bank=sparkasse`
- **THEN** no file is stored and no upload record is created

### Requirement: Every upload is recorded
The system SHALL keep one upload record per distinct file content, holding: an upload id, the file name, the bank, the time the upload was received (UTC), the hex SHA-256 hash of the body, the processing status, and, once done, the imported, already-known and pending-skipped counts. The file name SHALL be the `filename` request parameter, or empty when it is absent.

#### Scenario: Record of a finished upload
- **WHEN** `statement.csv` is posted to `/uploads/csv?bank=sparkasse&filename=statement.csv` and its import stores 5 rows, finds 3 known and skips 2 pending
- **THEN** one record exists with that file name, bank `sparkasse`, the receive time, the body's SHA-256, status `done` and counts 5, 3 and 2

#### Scenario: No file name given
- **WHEN** an export is posted without a `filename` parameter
- **THEN** its record's file name is empty

### Requirement: Upload status follows processing
An upload record SHALL be `processing` while its import runs, `done` when the import succeeded, and `failed` when the import returned an error, including an unknown bank. Counts SHALL be recorded only with `done`.

#### Scenario: Import fails
- **WHEN** an upload's import fails because storing expenses fails
- **THEN** its record's status is `failed`

#### Scenario: Unknown bank
- **WHEN** an export is posted to `/uploads/csv?bank=nobank` and no mapping named `nobank` exists
- **THEN** its record's status is `failed`

### Requirement: The content hash is the idempotency key
The system SHALL treat two uploads with the same SHA-256 body hash as the same upload, whatever their file name or bank. A repeat of a `done` upload SHALL neither store the file again nor import it, and SHALL be answered from the existing record.

#### Scenario: Same file posted twice
- **WHEN** an export is uploaded and finishes, then the identical bytes are posted again under a different `filename`
- **THEN** the second request imports nothing, no second record exists, and the answer carries the first upload's id and counts

### Requirement: Failed or abandoned uploads are processed again
A repeat of an upload whose record is `failed`, or `processing` since more than 15 minutes ago, SHALL be imported again under the same upload id, using the repeat request's bank and file name, and its record SHALL follow the status rules again. A repeat of an upload `processing` for 15 minutes or less SHALL be refused without importing.

#### Scenario: Retry after failure
- **WHEN** an upload failed with an unknown bank, and the same bytes are posted again with a known bank
- **THEN** the file is imported, and the same upload id ends in status `done` with the new bank

#### Scenario: Same file while it is processing
- **WHEN** a file is posted while an identical file's import started 1 minute ago and is still running
- **THEN** the second request imports nothing and is refused

#### Scenario: Abandoned processing
- **WHEN** an upload has been `processing` for 20 minutes because the server stopped mid-import, and the same file is posted again
- **THEN** the file is imported again under the same upload id
