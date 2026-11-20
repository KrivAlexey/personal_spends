# Proposal

## Why

An uploaded bank export is read once and thrown away: nothing records that a file was received, whether its import finished or failed, or lets the same file be recognized when it is posted again. Keeping the raw file in S3 and a metadata row per upload in PostgreSQL gives every upload an id, a status and a content-hash idempotency key, and starts the Postgres and S3 footprint that backlog items 2 (images in S3) and 4 (RDS PostgreSQL) build on.

## What Changes

- Every accepted `POST /uploads/csv` body is stored unchanged in S3, keyed by its SHA-256 hash.
- Every upload gets a row in a PostgreSQL `uploads` table: id, file name, bank, upload time, SHA-256 hash (unique, the idempotency key), processing status (`processing`, `done`, `failed`) and, once done, the import counts.
- Processing stays synchronous in the request: store file → record `processing` → import → record `done` with counts, or `failed`.
- Re-posting a file whose hash is already `done` returns `200` with that upload's id and original counts, without storing or importing anything. A hash whose upload `failed`, or is stuck in `processing` for longer than 15 minutes, is processed again. A hash that is `processing` right now answers `409`.
- New optional `filename` query parameter on `POST /uploads/csv` (the body is raw, ADR 0012, so there is no multipart file name).
- The `200` response gains an `upload_id` field. Existing fields are unchanged, so current clients keep working.
- Bodies over 10 MiB are rejected with `413` (the body is now held in memory to hash it before the S3 write; API Gateway caps payloads at 10 MB anyway).
- Local runs and tests: a `compose.yaml` starts DynamoDB Local, PostgreSQL and an S3 emulator; the S3 endpoint is overridden with `AWS_ENDPOINT_URL_S3`, the same way DynamoDB Local is. Unit tests stay offline with fakes; integration tests against the containers run only when their environment variables are set.
- Terraform gains the private uploads S3 bucket (`plan` only; `apply` waits for the owner's approval).
- Out of scope: RDS in Terraform (needs a VPC and costs money; stays backlog item 4, so until then Postgres runs locally only), async processing (backlog item 3), a `GET /uploads/{id}` endpoint, a migration tool.

## Capabilities

### New Capabilities
- `upload-tracking`: how an uploaded file is kept and recorded — raw file retention, the metadata record, its status lifecycle and content-hash idempotency.

### Modified Capabilities
- `rest-api`: `POST /uploads/csv` accepts `filename`, returns `upload_id`, answers repeated files from the stored result, and adds `409` (same file in progress) and `413` (body too large).

## Impact

- Code: new `internal/uploads` (orchestration: hash, S3 put, metadata record, import, status update) with S3 and Postgres implementations; `internal/handler` calls it instead of the importer directly; `cmd/server` wiring and config; `internal/handler/openapi.yaml`.
- Dependencies (new): `github.com/aws/aws-sdk-go-v2/service/s3`, `github.com/jackc/pgx/v5`.
- Config (new): `UPLOADS_BUCKET`, `DATABASE_URL`, `AWS_ENDPOINT_URL_S3`.
- Local infra: `compose.yaml`, `scripts/run-local.sh` / `.env.local` example, README setup.
- Terraform: `aws_s3_bucket` for uploads (private, encrypted). Plan only.
- Docs: `docs/architecture.md` (status, upload flow, data model, configuration, backlog), new ADR for S3 + PostgreSQL upload records and hash idempotency.
- No core interface (`Categorizer`, `CategoryProvider`, `BankMappingProvider`) changes. No real AWS resources are created by this change.
