# Architecture

## Overview

`personal_spends` is a Go backend that accepts bank CSV exports, categorizes expenses using Claude AI, and stores structured results in DynamoDB. It exposes a REST API (API Gateway + Lambda) and a local MCP server (stdio) for AI agent queries.

v1 is deliberately the smallest end-to-end path: CSV upload, serial categorization, DynamoDB write ([0007](decisions/0007-serial-loop-for-v1.md), [0009](decisions/0009-csv-only-v1.md)). The worker pool is step 2; receipt image extraction is backlog.

This document describes the v1 target. What is built so far:

## Implementation Status

| Area | State |
|------|-------|
| `internal/categories` | Built: `YamlCategoryProvider`, tested |
| `internal/categorizer` | Built: `Claude`, tool-output validation tested; live API test skipped without `ANTHROPIC_API_KEY` |
| `internal/parser` | Built: `Parser.ParseCSV` (pending filter, transaction IDs) + `FileMappingProvider`, tested |
| `internal/storage` | Built: `PutExpense`, `SaveExpenses` (batched, retried), `KnownIDs`, tested. Not yet: `QueryExpenses` |
| `internal/ingest` | Built: `Importer.Import` (serial loop, skips known rows), tested with fakes |
| `internal/handler`, `internal/mcp`, `cmd/*` | Not started |
| Terraform | DynamoDB table with GSI1 only. Not yet: Lambda, API Gateway |
| Configuration (below) | Not read anywhere yet; arrives with `cmd/` |

---

## System Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│  Local machine                                                  │
│                                                                 │
│  cmd/mcp  ◄──── Claude Code (stdio)                             │
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
│                                          │   └─ storage     │   │    (Claude Haiku)
│                                          │        │         │   │
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
POST /uploads/csv  (multipart, field: "file")
  │
  └─ ingest.Importer.Import(ctx, bankName, r)            the loop, testable without HTTP
        ├─ BankMappingProvider.GetMapping(bankName)  column mapping, encoding, delimiter, date and
        │                              decimal format, pending marker, identity columns
        ├─ Parser.ParseCSV(ctx, bankName, r)   drops pending rows, assigns each row its ID
        │                              over the whole file (0011)
        ├─ storage.KnownIDs(keys)      already stored → skipped, never re-categorized
        ├─ for each batch of ~50       serial loop in v1, worker pool in step 2 (0007)
        │     ├─ categorizer.Categorize(batch)   one Claude Haiku call per batch
        │     │     returns: category and confidence per transaction
        │     └─ storage.SaveExpenses()          batch write to DynamoDB
        └─ returns { imported, already_known, pending_skipped }
```

Re-uploading a file or an overlapping export imports only rows not stored before
([0011](decisions/0011-transaction-identity-and-deduplication.md); spec:
[`openspec/specs/csv-import`](../openspec/specs/csv-import/spec.md)). Everything below
`Import` is built; the HTTP handler calling it is not.

Bank mappings are written by hand and committed to the repo
(`internal/parser/bank_mappings.yaml`) — one entry per bank, added as a new
bank's export is encountered. An unknown `bankName` is an error, not a guess
([0008](decisions/0008-manual-bank-mappings.md)).
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
| `Categorizer` | `Claude` (Haiku) | `BedrockCategorizer` (Llama/Mistral), `VLLMCategorizer` (on-demand GPU) |
| `CategoryProvider` | `YamlCategoryProvider` | `PostgresProvider` (RDS + RDS Proxy) |
| `BankMappingProvider` | `FileMappingProvider` (`bank_mappings.yaml`) | Runtime detection with Claude |

Image extraction is intentionally absent from `Categorizer`. It comes back as
its own method when the S3 bucket and the Claude Vision call are built together
— an interface method with no implementation stops every implementation from
satisfying the interface ([0009](decisions/0009-csv-only-v1.md)).

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

---

## API Endpoints

All endpoints require `X-API-Key` header.

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/uploads/csv` | Upload bank export CSV |
| `GET` | `/expenses` | Query expenses (`from`, `to`, `category`, `min_amount`, `max_amount`, `limit` params) |
| `POST` | `/expenses` | Add a single expense manually (JSON body with the expense fields) |
| `GET` | `/expenses/summary` | Spending totals grouped by category (`from`, `to` params) |
| `GET` | `/categories` | List configured categories |

`min_amount` / `max_amount` filter the date- or category-keyed query result (a DynamoDB filter expression), so they never trigger a full scan.

Amounts are EUR only in v1; multi-currency is added if needed ([0010](decisions/0010-eur-only-v1.md)).

---

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
| `ANTHROPIC_API_KEY` | Anthropic API key | required |
| `API_KEY` | API key for endpoint auth | required |
| `DYNAMODB_TABLE` | Expenses table name | `personal-spends-expenses` |
| `AWS_REGION` | AWS region | `eu-central-1` |
| `BATCH_SIZE` | Transactions per Claude API call | `50` |
| `WORKER_POOL_SIZE` | Max concurrent categorizer goroutines (step 2) | `5` |

---

## Later Changes (the backlog, in order)

1. **Worker pool:** replace the serial batch loop with a bounded pool of goroutines fed by a channel, results collected before the DynamoDB write. First step after v1 runs end to end.
2. **Receipt / bill images:** `POST /uploads/image` stores to S3 (30-day lifecycle) and a Vision call extracts `[]Transaction`, which then takes the same path as CSV rows. Adds the S3 bucket to Terraform and a method back onto `Categorizer`.
3. **SQS fan-out:** S3 event on CSV upload triggers SQS. Lambda reads SQS batches instead of the whole file. Worker pool moves from intra-Lambda goroutines to parallel Lambda invocations.
4. **RDS PostgreSQL:** `CategoryProvider` swaps from YAML to Postgres. Adds `categories`, `vendor_rules`, and `budgets` tables. Lambda gets RDS Proxy for connection pooling. Requires VPC in Terraform.
5. **Bedrock:** `Categorizer` swaps from Anthropic SDK to `BedrockCategorizer` using `InvokeModelWithResponseStream`. Model ID configurable via env var, no changes in callers.
6. **MCP SSE transport:** make the MCP server reachable from claude.ai and other remote agents.
7. **On-demand GPU:** vLLM on spot EC2, brought up only during processing, for self-hosted model experiments (`VLLMCategorizer`).
8. **Runtime bank-mapping detection:** Claude reads the first N rows of an unknown bank's CSV and returns the column layout, persisted as a new mapping behind `BankMappingProvider` instead of hand-editing the YAML.
