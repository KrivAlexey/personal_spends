# 0015 — Uploaded files in S3, upload records in PostgreSQL, content hash as the idempotency key

**Status:** Accepted

## Context

`POST /uploads/csv` read the export once and threw it away (#123). Nothing recorded that a file arrived, whether its import finished or failed, or let the same file be recognized when posted again. Row-level dedup ([0011](0011-transaction-identity-and-deduplication.md)) already makes a re-import safe, but it still parses the file and looks up every row, and it can't tell "this exact file is being imported right now" from "this file is new". Backlog item 2 (receipt images) needs S3 anyway, and backlog item 4 brings PostgreSQL in for categories.

## Decision

- **Every accepted body is kept unchanged in S3**, at `csv/<sha256 hex>.csv`. Uploads rejected before import (missing `bank`, empty, unreadable or over 10 MiB) aren't kept.
- **Every distinct file content gets one row in a PostgreSQL `uploads` table**: id (UUID), file name (the optional `filename` query parameter), bank, upload time, SHA-256, status (`processing` → `done` | `failed`) and, once done, the import counts.
- **The SHA-256 of the body is the idempotency key** (unique column), whatever the bank or file name:
  - `done` → answered from the record, nothing kept or imported again
  - `processing` for 15 minutes or less → `409`
  - `failed`, or `processing` for more than 15 minutes (Lambda's maximum run time, so it can't still be running) → imported again under the same id, with the repeat's bank and file name
- **Claiming is one atomic upsert** (`INSERT … ON CONFLICT (sha256) DO UPDATE … WHERE failed OR stale RETURNING`), using the database clock. The conflicting row is locked for the upsert, so concurrent identical requests have one winner.
- **Processing stays synchronous** in the request: claim → keep file → import → finish or fail. The final status write runs on `context.WithoutCancel`, so a client disconnect can't leave the row `processing`.
- **`internal/uploads.Service`** orders the steps behind narrow, consumer-side interfaces; `S3Blobs` and `PostgresRecords` implement them, over `aws-sdk-go-v2/service/s3` and `jackc/pgx/v5`.
- **The schema is one embedded `schema.sql`**, applied at startup with `CREATE TABLE IF NOT EXISTS`. That's a shortcut: a migration tool comes with the first schema change `IF NOT EXISTS` can't express.
- **Local runs and tests** use `compose.yaml`: DynamoDB Local, `postgres:17.12-alpine` and Adobe S3Mock (`AWS_ENDPOINT_URL_S3`), which creates the bucket from an env var. The S3 bucket is in Terraform for real AWS only. Postgres has no AWS home until RDS (backlog item 4).

## Alternatives considered

| Option | Rejected because |
|---|---|
| Upload metadata in DynamoDB (already deployed) | Postgres is coming with backlog item 4 anyway, and the owner wants SQL practice; a conditional `PutItem` would also work for the claim |
| SQLite for metadata | No hosted equivalent for Lambda; it would be replaced later |
| Hash over bank + body | The owner chose the body alone: the same bytes are the same file. A wrong bank fails and is retried under the same id |
| `409` for every repeat | Clients would have to treat a repeat as an error; answering from the record makes the endpoint idempotent |
| Async processing (`202` + `GET /uploads/{id}`) | Backlog item 3 (SQS fan-out) changes the flow anyway; synchronous keeps one request one outcome |
| Streaming the body into S3 multipart while hashing | The key isn't known until the end, and exports are kilobytes; a 10 MiB in-memory cap is simpler |
| MinIO or LocalStack locally | MinIO's community images are no longer maintained; LocalStack needs an account token |

## Consequences

- `cmd/server` now needs `DATABASE_URL` and `UPLOADS_BUCKET`, so offline runs need the compose containers.
- The handler reads the whole body before importing. A body cut off mid-upload fails before anything is claimed or kept, which replaces the old read-error recorder.
- A crash between keeping the file and recording the outcome blocks that file's hash with `409` for up to 15 minutes.
- Error text of a failed upload isn't stored (it may contain bank data); the logs carry it with the `upload_id`.
- The Postgres and S3 integration tests run only locally (`TEST_DATABASE_URL`, `TEST_S3_ENDPOINT`). CI has no service containers yet.
- Nothing reads the kept files yet; a reprocess-from-S3 job, a retention lifecycle and RDS for `uploads` are later work.
