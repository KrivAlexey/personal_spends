# Tasks

## 1. Setup

- [x] 1.1 Open the GitHub issue, branch `feature/store-uploads-s3-postgres` off up-to-date `main`; verify `git status` shows the new branch
- [x] 1.2 Add `github.com/jackc/pgx/v5` and `github.com/aws/aws-sdk-go-v2/service/s3` (`go get`, `go mod tidy`); verify `go build ./...` passes
- [x] 1.3 Add `compose.yaml` (DynamoDB Local, `postgres:17`, Adobe S3Mock with the uploads bucket, all pinned image tags) (`.env.local` is gitignored, so the new variables are documented in the README instead, task 6.4); verify `docker compose up -d` starts all three and `aws --endpoint-url http://localhost:9090 s3 ls` lists the bucket (config exception: checked by running)

## 2. Upload orchestration (`internal/uploads.Service`)

- [x] 2.1 Failing table-driven tests with fakes for `blobStore`, `recordStore`, `importer`: a new upload claims, puts `csv/<sha256>.csv` and finishes with counts; a `done` repeat returns the stored id and counts without put or import; fresh `processing` gives `ErrInProgress`; a failed import, a failed put and an unknown bank each mark `failed` and keep the `parser.ErrBankMappingNotFound` wrap; `Fail` still runs when the request ctx is cancelled. Confirm they fail on assertions, commit them alone
- [x] 2.2 Implement `Service`, `Request`, `Result`, `ErrInProgress` and the DEBUG logs per design §1, §4, §9; verify the 2.1 tests pass, commit

## 3. Postgres upload records

- [x] 3.1 Failing integration tests for `PostgresRecords` (skipped unless `TEST_DATABASE_URL` is set): `EnsureSchema` twice is fine; first claim inserts `processing` with `uploaded_at`; a repeat of `done` returns no claim and the stored counts; `failed` is reclaimed under the same id with the new bank and file name; `processing` older than 15 minutes is reclaimed, newer is not; two concurrent claims of one hash yield exactly one owner. Run them against compose Postgres, confirm assertion failures, commit alone
- [x] 3.2 Implement `PostgresRecords` with embedded `schema.sql` and the upsert from design §3, `Finish`, `Fail`; verify 3.1 passes against compose Postgres and `go test ./...` passes without the env var, commit

## 4. S3 file store

- [x] 4.1 Failing tests for `S3Blobs.Put`: a unit test with a fake `s3API` checks bucket, key and exact body bytes, and passes on the client error wrapped; an integration test, skipped unless `TEST_S3_ENDPOINT` is set, puts then reads the object back from S3Mock. Confirm they fail on assertions, commit alone
- [x] 4.2 Implement `S3Blobs`; verify 4.1 passes (integration test against compose S3Mock), commit

## 5. REST endpoint

- [x] 5.1 Failing handler tests with a fake uploader, covering the rest-api delta scenarios: `200` with `upload_id` and all counts; the `filename` param passed through; a repeat returns the original `upload_id` and counts; `ErrInProgress` gives `409`; a 10,485,761-byte body gives `413` with no upload call, while exactly 10,485,760 bytes is accepted; empty, missing-bank, unreadable and cut-off bodies give `400` with no upload call; an unknown bank gives `400`; another error gives `500` without its text; the openapi routes and response-schema test covers `upload_id`, `filename`, 409 and 413. Confirm they fail on assertions, commit alone
- [x] 5.2 Switch the handler to the `uploader` interface: whole-body read with `MaxBytesReader`, remove `readErrRecorder`, new fixed rejection reasons, update `openapi.yaml`; verify 5.1 and the existing handler tests pass, commit

## 6. Wiring, infra and docs

- [x] 6.1 Wire `cmd/server`: `DATABASE_URL` (pgxpool + `EnsureSchema`), `UPLOADS_BUCKET`, S3 client with `UsePathStyle` when `AWS_ENDPOINT_URL_S3` is set, `uploads.Service` into the handler; verify by running `scripts/run-local.sh` against compose, posting `samples/` CSV twice (second answer has the same `upload_id`), and seeing the row `done` in `psql` and the object in S3Mock (thin-wiring exception)
- [x] 6.2 Terraform: uploads bucket (private, public access block, SSE-S3, `count = local.emulator ? 0 : 1`, name from the account id) and its permissions in `iam-policy.json`; verify `terraform fmt -check && terraform validate && terraform plan` shows only the bucket resources added. Do not apply
- [x] 6.3 ADR `0015` for S3 file retention, Postgres upload records and the content-hash idempotency key (and Postgres arriving before backlog item 4); verify it is linked from `docs/architecture.md`
- [x] 6.4 Update `docs/architecture.md` (Implementation Status, system diagram, upload flow, Postgres data model, API table, Configuration with `DATABASE_URL`, `UPLOADS_BUCKET`, `AWS_ENDPOINT_URL_S3`, backlog items 2 and 4) and README local setup (`docker compose up -d`, integration-test env vars); verify each doc section against the code
- [x] 6.5 Run `go build ./... && go vet ./... && go test ./...` and `openspec validate store-uploads-s3-postgres --strict`; verify both pass

## 7. Review

- [x] 7.1 Run the go-reviewer agent with `git diff main...HEAD`, `git log --oneline main..HEAD` and the build/vet/test output; fix its bug and risk findings (stop and ask if a fix needs a core interface change, a new dependency or a skipped test); verify `go test ./...` passes again

## Workflow follow-up

- Open the PR (template filled, Not verified lists the integration tests CI doesn't run and the S3Mock env var), then wait for `ci` with `gh pr checks <PR> --watch`
- Archive this change after implementation, then verify `openspec/specs/upload-tracking` and `openspec/specs/rest-api` hold the merged requirements
