# 0012 — CSV upload as a raw body, bank as a query parameter

**Status:** Accepted

## Context

`ingest.Importer.Import(ctx, bankName, r)` needs the bank name and the export, but `POST /uploads/csv` was documented with only a multipart `file` field and no way to name the bank (issue #77).

Uploads come from the owner via `curl`. The MCP tools only query, so nothing uploads from them, and there is no browser form.

## Decision

`POST /uploads/csv?bank=<bankName>` with the export as the raw request body (`Content-Type: text/csv`). The handler passes the body to `Import` byte for byte. Decoding the bank's encoding stays in the parser.

- Missing or empty `bank` → `400`
- Empty body, or a body that fails to read (connection dropped before or during the upload) → `400`
- `parser.ErrBankMappingNotFound` → `400` (an unknown bank is an error, not a guess: [0008](0008-manual-bank-mappings.md))
- Any other import error → `500`, with no internal error text in the response
- Success → `200` with `{"imported", "already_known", "pending_skipped"}`

## Alternatives considered

| Option | Rejected because |
|---|---|
| Multipart form (`bank` and `file` fields) | Adds form parsing (memory and temp-file spill) and two extra error cases ("not multipart", "no file part") for a browser form that doesn't exist |
| Detect the bank from the CSV header | Needs a header signature per bank mapping, and an ambiguous header would be a guess, against 0008 |

## Consequences

- Upload with `curl --data-binary @export.csv` (not `-d`, which strips newlines).
- A future web UI that uploads through a `<form>` adds multipart then, next to the raw body.
