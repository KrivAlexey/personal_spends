# Design

## Context

`POST /uploads/csv` streams the raw body ([0012](../../../docs/decisions/0012-raw-csv-upload-body.md)) straight into `ingest.Importer.Import`. `handler.readErrRecorder` exists only to tell a client read error apart from an import error mid-stream. Row-level dedup ([0011](../../../docs/decisions/0011-transaction-identity-and-deduplication.md)) already makes re-imports safe; this change adds file-level identity on top of it, it does not replace it. Local runs use DynamoDB Local plus the Terraform `local` workspace, and the SDK picks up `AWS_ENDPOINT_URL_DYNAMODB`. Requirements: `specs/upload-tracking` and `specs/rest-api`.

## Goals / Non-Goals

**Goals:**
- One place (`internal/uploads`) that orders hash → claim → keep file → import → record outcome, testable with fakes.
- Claiming a hash is atomic in Postgres, so two identical requests can never both import.
- `go run ./cmd/server` works offline against containers from one `docker compose up`.

**Non-Goals:**
- Reading files back from S3 (no reprocess-from-S3 job yet).
- Storing the error text of a failed upload (it may contain bank data; logs already carry it).
- Running Postgres in AWS, a migration tool, CI service containers (the agent's token can't edit `.github/workflows`; owner's call later).

## Decisions

**1. New package `internal/uploads` with a `Service`; the handler depends on it, not on the importer.**
`Service.Upload(ctx, Request{Bank, Filename string; Body []byte}) (Result, error)`, where `Result` is `ingest.Result` plus `UploadID`. Its dependencies are unexported, consumer-side interfaces (per CLAUDE.md and golang-structs-interfaces): `blobStore` (`Put(ctx, key string, body []byte) error`), `recordStore` (`Claim`, `Finish`, `Fail`), and `importer` (the existing `Import` signature). Implementations `S3Blobs` and `PostgresRecords` live in the same package, each over its own narrow client interface (`s3API` over `*s3.Client`, a `pgx` pool subset), like `storage.dynamoDBAPI`.
*Alternative:* put the logic into `ingest.Importer`. Rejected: the importer's contract is "parse and store rows"; mixing file retention into it would grow its fakes and tests for no gain.

**2. The handler reads the whole body, capped at 10 MiB, before anything else.**
`http.MaxBytesReader` + `io.ReadAll`; a `*http.MaxBytesError` maps to `413`, other read errors to `400`, zero bytes to `400`. The body must be fully read to hash it before the S3 key is known, and `PutObject` wants a known length anyway. This removes `readErrRecorder`: a cut-off body now fails in `ReadAll`, before any claim or import. The importer gets `bytes.NewReader(body)`, so the bytes stay unchanged (rest-api "passed through unchanged").
*Alternative:* stream through a `TeeReader` into a hasher and S3 multipart upload. Rejected: the key isn't known until the end, and exports are kilobytes.

**3. Idempotency is one atomic upsert in Postgres, using the database clock.**
```sql
INSERT INTO uploads (sha256, filename, bank) VALUES ($1, $2, $3)
ON CONFLICT (sha256) DO UPDATE
   SET status = 'processing', filename = EXCLUDED.filename, bank = EXCLUDED.bank,
       started_at = now(), imported = NULL, already_known = NULL, pending_skipped = NULL
 WHERE uploads.status = 'failed'
    OR (uploads.status = 'processing' AND uploads.started_at < now() - interval '15 minutes')
RETURNING id
```
A returned row means this request owns the import. No row means the existing record is `done` (answer from it) or fresh `processing` (`ErrInProgress` → `409`); a follow-up `SELECT` by `sha256` tells which. The conflicting row is locked during the upsert, so concurrent identical requests serialize. 15 minutes is Lambda's maximum timeout, so a `processing` row older than that cannot still be running.
*Alternative:* `SELECT … FOR UPDATE` then branch in Go inside a transaction. Same result, more round trips and code.

**4. Order inside `Upload`: claim → put → import → finish/fail.**
Claim first so a `done` repeat writes nothing to S3. The S3 key is `csv/<sha256 hex>.csv`; a retry puts the same bytes to the same key, which is harmless. A failed put marks the record `failed` and returns a 500-class error. `Finish`/`Fail` run on `context.WithoutCancel(ctx)` with a 5 s timeout, so a client disconnect can't leave the row `processing` (that would block retries for 15 minutes). The `parser.ErrBankMappingNotFound` wrap is kept, so the handler's `400` mapping still works.

**5. Schema: one embedded `schema.sql`, applied at startup with `CREATE TABLE IF NOT EXISTS`.**
```sql
CREATE TABLE IF NOT EXISTS uploads (
  id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
  sha256          char(64)    NOT NULL UNIQUE,
  filename        text        NOT NULL DEFAULT '',
  bank            text        NOT NULL,
  uploaded_at     timestamptz NOT NULL DEFAULT now(),
  started_at      timestamptz NOT NULL DEFAULT now(),
  status          text        NOT NULL DEFAULT 'processing'
                  CHECK (status IN ('processing', 'done', 'failed')),
  imported        integer,
  already_known   integer,
  pending_skipped integer
);
```
`gen_random_uuid()` is built into Postgres 13+, so no UUID dependency. `PostgresRecords.EnsureSchema(ctx)` runs it from `cmd/server`. This is a `shortcut:` with a known limit, upgraded to a migration tool on the first schema change that `IF NOT EXISTS` can't express.

**6. Drivers: `jackc/pgx/v5` (`pgxpool`) and `aws-sdk-go-v2/service/s3`.**
pgx is the de facto Postgres driver and needs no `database/sql` layer for one table. The S3 client comes from the AWS config `cmd/server` already loads. With `AWS_ENDPOINT_URL_S3` set, the SDK targets the emulator; `cmd/server` also sets `UsePathStyle` then, because emulators don't serve virtual-host bucket names.

**7. Local stack: `compose.yaml` with DynamoDB Local, `postgres:17` and Adobe S3Mock.**
S3Mock is Apache-licensed, built for tests, and creates the bucket from an env var at startup. MinIO's community edition no longer ships maintained images, and LocalStack needs an account token, so neither fits. The Terraform `local` workspace keeps creating the DynamoDB table only. The uploads bucket resource uses `count = local.emulator ? 0 : 1`, because provider refreshes of S3 sub-resources (ACL, CORS, versioning) aren't reliably supported by emulators.

**8. Tests.**
`uploads.Service` gets table-driven tests with in-memory fakes covering every status path. `PostgresRecords` and `S3Blobs` get integration tests against the compose containers, skipped unless `TEST_DATABASE_URL` / `TEST_S3_ENDPOINT` are set, like the `ANTHROPIC_API_KEY` skip. The claim race is tested there with two goroutines claiming one hash. Handler tests use a fake uploader for 200/400/409/413 and the repeat response.

**9. Logging follows 0013.**
New handler rejections use fixed reasons (`"body too large"`, `"upload in progress"`). `Service` logs DEBUG `upload claimed` / `upload repeated` with `upload_id` and `status`. Neither file name nor hash is logged: the file name is caller input, and a hash of a bank file is not needed for operations.

## Risks / Trade-offs

- [The S3 put succeeds but the server dies before `Finish`/`Fail`] → the row stays `processing` and its hash answers `409` for up to 15 minutes. After that a repeat retakes it. Acceptable for one user.
- [The same bytes are posted with a different bank] → treated as the same upload, per spec. A `done` repeat returns the first bank's result, which is what the owner chose.
- [Postgres is required to run `cmd/server`, but has no AWS home yet] → `cmd/lambda` isn't built, so nothing deployed breaks. Backlog item 4 (RDS) now also carries `uploads`; docs/architecture.md says so.
- [CI can't run the integration tests (no service containers)] → they run locally with compose. The PR's **Not verified** section says so.
- [10 MiB held in memory per request] → bounded by `MaxBytesReader`, and API Gateway caps payloads at 10 MB anyway.
- [S3Mock's env var for initial buckets differs between major versions] → the compose file pins an exact image tag, checked when running it.

## Migration Plan

No existing data. Start the containers (`docker compose up -d`), then run `go run ./cmd/server`, which creates the `uploads` table. The real S3 bucket appears in `terraform plan` only; `apply` waits for the owner. Rollback: revert the PR. The table and bucket can stay, since nothing else reads them.
