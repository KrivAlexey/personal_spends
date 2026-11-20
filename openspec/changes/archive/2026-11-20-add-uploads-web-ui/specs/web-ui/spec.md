# Spec Delta

## Purpose

A browser page, served by the API server itself, for uploading bank exports and seeing the record of every upload: what was sent, when, under which bank, and how its import ended.

## ADDED Requirements

### Requirement: The page is served at the root
The system SHALL respond to `GET /` with `200` and an HTML page (`Content-Type: text/html; charset=utf-8`) that loads its script from `/app.js` and its styles from `/app.css`, both served by the same server. No part of the page SHALL be loaded from another origin.

#### Scenario: Open the page
- **WHEN** a browser opens `http://localhost:8080/`
- **THEN** the response is `200` HTML, and the page loads `/app.js` (`text/javascript`) and `/app.css` (`text/css`) from the same server

#### Scenario: Unknown path is not the page
- **WHEN** a client sends `GET /nothing-here`
- **THEN** the response is `404`, not the page

### Requirement: Page responses forbid foreign and inline scripts
Responses to `GET /`, `GET /app.js` and `GET /app.css` SHALL carry `Content-Security-Policy: default-src 'self'; frame-ancestors 'none'` and `X-Content-Type-Options: nosniff`, so the browser runs no inline or foreign script and no other site can frame the page.

#### Scenario: Security headers
- **WHEN** a client sends `GET /`
- **THEN** the response has `Content-Security-Policy: default-src 'self'; frame-ancestors 'none'` and `X-Content-Type-Options: nosniff`

### Requirement: The page lists uploads
The page SHALL show the uploads returned by `GET /uploads`, newest first. Each row shows the file name, bank, upload time in the browser's local time zone, and status. A `done` upload also shows its imported, already-known and pending-skipped counts. An empty list SHALL show a "no uploads yet" message instead of an empty table.

#### Scenario: Done and failed uploads
- **WHEN** the page is opened after `statement.csv` finished with counts 5, 3 and 2, and a later upload `wrong.csv` failed
- **THEN** the table's first row is `wrong.csv` with status `failed` and no counts, and the second is `statement.csv` with status `done` and counts 5, 3 and 2

#### Scenario: No uploads
- **WHEN** the page is opened and no upload was ever recorded
- **THEN** the page shows "no uploads yet"

### Requirement: The page uploads a file
The page SHALL offer a bank dropdown filled from `GET /banks`, a file picker and an Upload button. Upload SHALL send the chosen file's bytes unchanged as the body of `POST /uploads/csv`, with the selected bank as `bank` and the file's name as `filename`. When it answers `200`, the page SHALL show the returned `upload_id` and counts and refresh the upload list.

#### Scenario: Upload an export
- **WHEN** the user selects bank `sparkasse`, picks `samples/sparkasse_fake.csv` and clicks Upload
- **THEN** the server receives that file's exact bytes with `bank=sparkasse` and `filename=sparkasse_fake.csv`, the page shows the `upload_id` and counts, and the new upload appears at the top of the list (a retry of an earlier upload keeps that upload's place, as `rest-api` specifies)

#### Scenario: Upload without a file
- **WHEN** the user clicks Upload with no file picked
- **THEN** no request is sent and the page asks for a file

### Requirement: The page shows upload errors
When `POST /uploads/csv` answers anything other than `200`, or the request fails, the page SHALL show the status code and response text near the form, keep the chosen file and bank selected, and refresh the upload list.

#### Scenario: Same file still importing
- **WHEN** an upload is answered `409`
- **THEN** the page shows the 409 and its message, and the form keeps its bank and file

#### Scenario: File too large
- **WHEN** an upload is answered `413`
- **THEN** the page shows the 413 and its message

### Requirement: Caller-supplied text is never interpreted as markup
The page SHALL display file names, bank names and server messages as plain text, so that no value from an upload or a response can add elements, attributes or script to the page.

#### Scenario: Markup in a file name
- **WHEN** a file named `<img src=x onerror=alert(1)>.csv` was uploaded and the page lists it
- **THEN** the row shows the literal text `<img src=x onerror=alert(1)>.csv`, and no image element or script is added
