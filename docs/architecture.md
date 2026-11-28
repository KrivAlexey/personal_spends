# Architecture

## Overview

`personal_spends` is a Go backend that accepts bank CSV exports, keeps each uploaded file in S3 with a record in PostgreSQL, categorizes expenses using Claude AI, and stores structured results in DynamoDB. It exposes a REST API (API Gateway + Lambda) and a local MCP server (stdio) for AI agent queries.

v1 is deliberately the smallest end-to-end path: CSV upload, serial categorization, DynamoDB write ([0007](decisions/0007-serial-loop-for-v1.md), [0009](decisions/0009-csv-only-v1.md)). The worker pool is step 2; receipt image extraction is backlog.

This document describes the v1 target. What is built so far:

## Implementation Status

| Area | State |
|------|-------|
| `internal/categories` | Built: `YamlCategoryProvider`, tested |
| `internal/categorizer` | Built: `Claude`, tool-output validation tested; live API test skipped without `ANTHROPIC_API_KEY`. `Keyword` (rule-based, no model) for offline local runs, tested |
| `internal/parser` | Built: `Parser.ParseCSV` (pending filter, transaction IDs) + `FileMappingProvider` (with `Banks()` for the UI), tested |
| `internal/storage` | Built: `SaveExpenses` (batched, retried), `KnownIDs`, `QueryExpenses` (base table by date, GSI1 by category, amount bounds as a filter expression, never a scan), tested with a fake client |
| `internal/ingest` | Built: `Importer.Import` (serial loop, skips known rows), tested with fakes |
| `internal/uploads` | Built: `Service.Upload` (claim by SHA-256, keep the file, import, record the outcome), `Service.List`, `PostgresRecords`, `S3Blobs`, tested with fakes; Postgres and S3 integration tests run against the compose containers only ([0015](decisions/0015-upload-files-and-records.md)) |
| `internal/handler` | Built: `POST /uploads/csv` ([0012](decisions/0012-raw-csv-upload-body.md), over `uploads.Service`), `GET /uploads`, `GET /banks`, `GET /expenses`, `POST /expenses`, `GET /expenses/summary`, `GET /categories`, `GET /openapi.yaml` ([0014](decisions/0014-hand-written-openapi-spec.md)), the upload web page at `/` ([0016](decisions/0016-embedded-static-web-ui.md)), request ID + access log + panic recovery middleware, tested with fakes. Not yet: `X-API-Key` middleware |
| `internal/logging` | Built: `New` (level/format), context attributes (`request_id`, `batch`, `batches`), tested ([0013](decisions/0013-structured-logging-with-slog.md)) |
| `cmd/server` | Built: wires the CSV upload path and the expense and category endpoints, serves `localhost:8080`, graceful shutdown on SIGINT/SIGTERM, creates the `uploads` table at startup. `CATEGORIZER=fake` + the `compose.yaml` containers run it offline (README). Checked by running it |
| `internal/mcp`, `cmd/lambda`, `cmd/mcp` | Not started |
| Terraform | DynamoDB table with GSI1; uploads S3 bucket (real AWS only, S3Mock locally); workspace `local` creates the table in DynamoDB Local. Not yet: Lambda, API Gateway, and with them the VPC, RDS PostgreSQL and RDS Proxy for `uploads` ([0017](decisions/0017-rds-postgres-for-uploads.md)) |
| Configuration (below) | Read by `cmd/server`, except `API_KEY` (no auth middleware yet) and `WORKER_POOL_SIZE` (step 2) |

---

## System Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│  Local machine                                                  │
│                                                                 │
│  cmd/mcp  ◄──── Claude Code (stdio)                             │
│  browser  ────► cmd/server  GET / (upload page, 0016)           │
│     │                                                           │
│     └──── HTTPS + API key ─────────────────────────┐           │
│                                                     │           │
│  cmd/server  (dev only, localhost:8080)             │           │
│                                                     ▼           │
│                                          ┌──────────────────┐   │
│                                          │  AWS             │   │
│                                          │                  │   │
│                                          │  API Gateway     │   │
│                                          │       │          │   │
│                                          │       ▼          │   │
│                                          │  Lambda          │   │
│                                          │  cmd/lambda      │   │
│                                          │   ├─ handler     │   │
│                                          │   ├─ parser      │   │
│                                          │   ├─ categorizer─┼───┼──► Anthropic API
│                                          │   ├─ storage     │   │    (Claude Haiku)
│                                          │   └─ uploads ────┼───┼──► S3 (files), PostgreSQL
│                                          │        │         │   │    (records; local until RDS)
│                                          │        ▼         │   │
│                                          │  DynamoDB        │   │
│                                          └──────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

---

## Entry Points

Three thin `main.go` files, all wiring up the same `internal/` packages:

| Entry point | Purpose |
|-------------|---------|
| `cmd/lambda/` | Production — Lambda handler deployed to AWS |
| `cmd/server/` | Development — plain HTTP server on `localhost:8080` |
| `cmd/mcp/` | Local MCP server — communicates via stdio with Claude Code |

---

## Request Flows

### CSV upload

```
POST /uploads/csv?bank=<bankName>[&filename=<name>]  (raw CSV body, 0012)
  │
  ├─ read the whole body (max 10 MiB, else 413; empty or unreadable → 400)
  └─ uploads.Service.Upload(ctx, {bank, filename, body})       0015
        ├─ PostgresRecords.Claim(sha256(body))   atomic upsert on the unique hash:
        │     done → return its id + counts      new, failed or stale (>15 min) processing
        │     processing → 409                   → claimed as processing
        ├─ S3Blobs.Put(csv/<sha256>.csv, body)          ┐ one 13 min deadline,
        ├─ ingest.Importer.Import(ctx, bankName, r)     ┘ the loop below
        └─ Finish(counts) → done, or Fail → failed  (survives a client disconnect;
                                                    only while this claim holds)

ingest.Importer.Import(ctx, bankName, r)                  the loop, testable without HTTP
        ├─ BankMappingProvider.GetMapping(bankName)  column mapping, encoding, delimiter, date and
        │                              decimal format, pending marker, identity columns
        ├─ Parser.ParseCSV(ctx, bankName, r)   drops pending rows, assigns each row its ID
        │                              over the whole file (0011)
        ├─ storage.KnownIDs(keys)      already stored → skipped, never re-categorized
        ├─ for each batch of ~50       serial loop in v1, worker pool in step 2 (0007)
        │     ├─ categorizer.Categorize(batch)   one Claude Haiku call per batch
        │     │     returns: category and confidence per transaction
        │     └─ storage.SaveExpenses()          batch write to DynamoDB
        └─ returns { imported, already_known, pending_skipped }  (+ upload_id from Upload)
```

Re-posting the identical bytes is answered from the upload record without importing
([0015](decisions/0015-upload-files-and-records.md); spec:
[`openspec/specs/upload-tracking`](../openspec/specs/upload-tracking/spec.md)). An
overlapping export, or a failed upload retried, imports only rows not stored before
([0011](decisions/0011-transaction-identity-and-deduplication.md); spec:
[`openspec/specs/csv-import`](../openspec/specs/csv-import/spec.md)). Everything below
`Import` is built, and so is the handler calling it; `cmd/server` serves it locally.

Bank mappings are written by hand and committed to the repo
(`internal/parser/bank_mappings.yaml`) — one entry per bank, added as a new
bank's export is encountered. An unknown `bankName` is an error, not a guess
([0008](decisions/0008-manual-bank-mappings.md)).
Every mapped column must be in an export's header, except those listed under
`optionalColumns` (never `Date` or `Amount`): their field stays empty when the
column is absent.
Generating a mapping at runtime (Claude reads the first N rows and returns the
column layout, result persisted as a new mapping) comes later; the
`BankMappingProvider` interface is the seam for it.

### MCP query (local)

```
Claude Code  ──stdio──►  cmd/mcp
                              │
                              └─ HTTPS + API key ──► API Gateway ──► Lambda
                                                                        │
                                                              storage.QueryExpenses()
                                                                        │
                                                                   DynamoDB
```

---

## Core Interfaces

These three domain interfaces each have more than one planned implementation, so each lives in its domain package next to the types and sentinel errors its implementations share (like `io.Reader`). Narrow interfaces a consumer needs only for itself are unexported in the consuming package, e.g. `storage.dynamoDBAPI` over the AWS client. Callers depend on the interface; constructors return concrete types.

```go
// internal/categorizer/categorizer.go
type Categorizer interface {
    Categorize(ctx context.Context, batch []Transaction) ([]CategorizedTransaction, error)
}

// internal/categories/provider.go
type CategoryProvider interface {
    List(ctx context.Context) ([]Category, error)
    Get(ctx context.Context, name string) (Category, error)
}

// internal/parser/mapping.go
type BankMappingProvider interface {
    GetMapping(ctx context.Context, bankName string) (BankMapping, error)
}
```

**Implementations:**

| Interface | v1 | Later (backlog) |
|-----------|----|-----------------|
| `Categorizer` | `Claude` (Haiku); `Keyword` for offline local runs | `BedrockCategorizer` (Llama/Mistral), `VLLMCategorizer` (on-demand GPU) |
| `CategoryProvider` | `YamlCategoryProvider` | `PostgresProvider` (RDS + RDS Proxy) |
| `BankMappingProvider` | `FileMappingProvider` (`bank_mappings.yaml`) | Runtime detection with Claude |

Image extraction is intentionally absent from `Categorizer`. It comes back as
its own method when image uploads and the Claude Vision call are built together
— an interface method with no implementation stops every implementation from
satisfying the interface ([0009](decisions/0009-csv-only-v1.md)).

Every `Categorizer` writes one DEBUG record per batch (`size`, `duration_ms`, `input_tokens`, `output_tokens`, `stop_reason`; `0`/`none` for an implementation that calls no model), specified in [`openspec/specs/categorizer`](../openspec/specs/categorizer/spec.md). Its tests call `categorizertest.AssertBatchLogging`.

---

## Data Model

### DynamoDB — `expenses` table

Single-table design. Access patterns drive the key structure.

**Base table**

| Key | Type | Value |
|-----|------|-------|
| `PK` | Partition key | `EXPENSES` |
| `SK` | Sort key | `<YYYY-MM-DD>#<id>` — booking date, enables date range queries; `<id>` is the deterministic transaction ID from [0011](decisions/0011-transaction-identity-and-deduplication.md) |

**Attributes per item**

| Attribute | Type | Example |
|-----------|------|---------|
| `amount` | Number | `42.50` |
| `currency` | String | `EUR` |
| `merchant` | String | `REWE` |
| `raw_description` | String | Original bank text |
| `category` | String | `groceries` |
| `confidence` | Number | `0.92` |
| `source` | String | Bank name from the mapping, e.g. `sparkasse` |
| `created_at` | String | ISO 8601 |

**GSI1 — category queries**

| Key | Value |
|-----|-------|
| `GSI1PK` | `CAT#<category>` e.g. `CAT#groceries` |
| `GSI1SK` | `<YYYY-MM-DD>#<id>` (same as `SK`) |

Enables: "show all grocery expenses in April", "sum transport costs for Q1".

### PostgreSQL — `uploads` table

One row per distinct uploaded file content ([0015](decisions/0015-upload-files-and-records.md)). Schema: `internal/uploads/schema.sql`, applied at startup with `CREATE TABLE IF NOT EXISTS`.

| Column | Type | Value |
|--------|------|-------|
| `id` | `uuid` PK | `gen_random_uuid()`, returned as `upload_id` |
| `sha256` | `char(64)` unique | Hex SHA-256 of the body, the idempotency key |
| `filename` | `text` | `filename` query parameter, `''` when absent |
| `bank` | `text` | `bank` query parameter of the latest claim |
| `uploaded_at` | `timestamptz` | First receipt |
| `started_at` | `timestamptz` | Start of the latest import; a `processing` row older than 15 minutes can be reclaimed |
| `status` | `text` | `processing`, `done` or `failed` |
| `imported`, `already_known`, `pending_skipped` | `integer` | Import counts, set only when `done` |

### S3 — uploads bucket

Each kept file is at `csv/<sha256 hex>.csv`, unchanged bytes, `Content-Type: text/csv`. No lifecycle rule yet.

---

## API Endpoints

All endpoints require `X-API-Key` header. Built endpoints are specified in [`openspec/specs/rest-api`](../openspec/specs/rest-api/spec.md) and described for clients in the OpenAPI 3.1 document [`internal/handler/openapi.yaml`](../internal/handler/openapi.yaml), served at `GET /openapi.yaml`; a test fails when it and the registered routes differ ([0014](decisions/0014-hand-written-openapi-spec.md)).

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/uploads/csv` | Upload bank export CSV as the raw body (`bank` param, required; `filename` optional); returns `upload_id`, `imported`, `already_known`, `pending_skipped`. Identical bytes again: the first upload's result. `409` while the same bytes are importing, `413` over 10 MiB ([0012](decisions/0012-raw-csv-upload-body.md), [0015](decisions/0015-upload-files-and-records.md)) |
| `GET` | `/expenses` | Query expenses, newest first (`from`, `to` inclusive dates; `category`; `min_amount`, `max_amount`; `limit`, default 100, at most 500) |
| `POST` | `/expenses` | Add a single expense manually: JSON `date`, `amount`, `merchant`, `category` (must be configured), optional `raw_description`. The server sets `currency` EUR, `confidence` 1, `source` `manual` and a random `id`; answers `201` with the expense. Stored with `SaveExpenses` |
| `GET` | `/expenses/summary` | Spending totals grouped by category over every expense in the range (`from`, `to` params), amounts rounded to cents |
| `GET` | `/categories` | List configured categories |
| `GET` | `/uploads` | The 100 most recent upload records, newest first: `upload_id`, `filename`, `bank`, `uploaded_at`, `status`, counts (`null` unless done) |
| `GET` | `/banks` | Configured bank mapping names, sorted, the values `bank` accepts |
| `GET` | `/openapi.yaml` | This API's OpenAPI 3.1 document |
| `GET` | `/`, `/app.js`, `/app.css` | The upload web page and its script and styles ([0016](decisions/0016-embedded-static-web-ui.md)) |

`min_amount` / `max_amount` filter the date- or category-keyed query result (a DynamoDB filter expression), so they never trigger a full scan.

Amounts are EUR only in v1; multi-currency is added if needed ([0010](decisions/0010-eur-only-v1.md)).

---

## Web UI

`GET /` serves a single page, embedded in the binary from `internal/handler/web/` ([0016](decisions/0016-embedded-static-web-ui.md); spec: [`openspec/specs/web-ui`](../openspec/specs/web-ui/spec.md)).

- **Upload form:** a bank dropdown filled from `GET /banks`, a file picker and Upload. The file's bytes go unchanged to `POST /uploads/csv?bank=…&filename=…`, and the `upload_id` and counts or the error are shown.
- **Uploads table:** from `GET /uploads`. It reloads after every upload and on Refresh.

The page files are served with `Content-Security-Policy: default-src 'self'; frame-ancestors 'none'`, and the script builds the DOM from text only, so a file name can't become markup. Screenshots of a run are in [`docs/screenshots/web-ui/`](screenshots/web-ui/).

## MCP Tools (v1)

| Tool | Description |
|------|-------------|
Each tool is a thin client over one REST endpoint ([0005](decisions/0005-mcp-server-local.md)).

| Tool | Description | Endpoint |
|------|-------------|----------|
| `get_expenses` | Query expenses by date range, category, or amount | `GET /expenses` |
| `get_summary` | Spending totals grouped by category for a period | `GET /expenses/summary` |
| `list_categories` | List all configured categories | `GET /categories` |
| `add_expense` | Manually add a single expense record | `POST /expenses` |

---

## Worker Pool Design (step 2, not in v1)

The plan for parallelizing the Claude API calls in CSV processing (I/O-bound,
[0003](decisions/0003-worker-pool-for-csv.md)). v1 walks the batches in a serial
loop instead ([0007](decisions/0007-serial-loop-for-v1.md)) — the first upload is
also the first time handler, parser, categorizer and storage run together, and a
serial loop is far easier to debug. The pool replaces the loop once that path works.

```
CSV rows (N transactions)
    │
    ▼
split into batches of BATCH_SIZE (default 50)
    │
    ├─ batch 1 ──► goroutine ──► categorizer.Categorize() ──► results chan
    ├─ batch 2 ──► goroutine ──► categorizer.Categorize() ──► results chan
    ├─ batch 3 ──► goroutine ──► categorizer.Categorize() ──► results chan
    └─ ...  (max WORKER_POOL_SIZE concurrent goroutines, default 5)
    │
    ▼
collect from results chan ──► storage.SaveExpenses()
```

---

## Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `ANTHROPIC_API_KEY` | Anthropic API key | required unless `CATEGORIZER=fake` |
| `CATEGORIZER` | `claude`, or `fake` for the rule-based `Keyword` categorizer (no model, local only) | `claude` |
| `API_KEY` | API key for endpoint auth | required |
| `DYNAMODB_TABLE` | Expenses table name | `personal-spends-expenses` |
| `AWS_REGION` | AWS region | `eu-central-1` |
| `AWS_ENDPOINT_URL_DYNAMODB` | DynamoDB endpoint override, read by the AWS SDK; `http://localhost:8000` for DynamoDB Local | unset (real AWS) |
| `UPLOADS_BUCKET` | S3 bucket for uploaded files (Terraform output `uploads_bucket_name`; `personal-spends-uploads` in S3Mock) | required |
| `AWS_ENDPOINT_URL_S3` | S3 endpoint override, read by the AWS SDK; `http://localhost:9090` for S3Mock. When set, `cmd/server` also uses path-style addressing | unset (real AWS) |
| `DATABASE_URL` | PostgreSQL connection string for upload records, e.g. `postgres://spends:spends@localhost:5432/spends` (compose) | required |
| `BATCH_SIZE` | Transactions per Claude API call | `50` |
| `WORKER_POOL_SIZE` | Max concurrent categorizer goroutines (step 2) | `5` |
| `LOG_LEVEL` | `debug`, `info`, `warn` or `error` (case-insensitive) | `info` |
| `LOG_FORMAT` | `text` or `json` (`json` on Lambda, for CloudWatch) | `text` |

---

## Logging

`log/slog`, one logger built by `logging.New` and injected into the handler, importer, categorizer and store ([0013](decisions/0013-structured-logging-with-slog.md)). What is logged is specified in [`openspec/specs/logging`](../openspec/specs/logging/spec.md):

- INFO: one access record per request (`method`, `route`, `status`, `duration_ms`), never the raw path
- WARN: `400`, `409` and `413` rejections (fixed `reason`), client disconnects, DynamoDB retries
- ERROR: `500`s, with the full error chain and `upload_id`; handler panics, with the stack
- DEBUG: upload claimed or repeated (`upload_id`, `status`), parse, dedup, per-batch categorize and save, and the import summary

Every record logged while serving a request carries `request_id`, added from the context by the `logging` handler wrapper together with `batch`/`batches`. Errors are logged once, at the handler. No bank data reaches the logs at any level.

---

## Later Changes (the backlog, in order)

1. **Worker pool:** replace the serial batch loop with a bounded pool of goroutines fed by a channel, results collected before the DynamoDB write. First step after v1 runs end to end.
2. **Receipt / bill images:** `POST /uploads/image` stores to the uploads S3 bucket (built for CSVs, [0015](decisions/0015-upload-files-and-records.md); 30-day lifecycle on images) and a Vision call extracts `[]Transaction`, which then takes the same path as CSV rows. Adds a method back onto `Categorizer`.
3. **SQS fan-out:** S3 event on CSV upload triggers SQS. Lambda reads SQS batches instead of the whole file. Worker pool moves from intra-Lambda goroutines to parallel Lambda invocations.
4. **RDS PostgreSQL:** the database itself (VPC, private RDS instance, RDS Proxy) comes earlier, with `cmd/lambda`, because the deployed upload path needs the `uploads` table ([0017](decisions/0017-rds-postgres-for-uploads.md)). This step is what remains: `CategoryProvider` swaps from YAML to Postgres, adding the `categories`, `vendor_rules` and `budgets` tables to the same database.
5. **Bedrock:** `Categorizer` swaps from Anthropic SDK to `BedrockCategorizer` using `InvokeModelWithResponseStream`. Model ID configurable via env var, no changes in callers.
6. **MCP SSE transport:** make the MCP server reachable from claude.ai and other remote agents.
7. **On-demand GPU:** vLLM on spot EC2, brought up only during processing, for self-hosted model experiments (`VLLMCategorizer`).
8. **Runtime bank-mapping detection:** Claude reads the first N rows of an unknown bank's CSV and returns the column layout, persisted as a new mapping behind `BankMappingProvider` instead of hand-editing the YAML.
