# Architecture

## Overview

`personal_spends` is a Go backend that accepts bank CSV exports, categorizes expenses using Claude AI, and stores structured results in DynamoDB. It exposes a REST API (API Gateway + Lambda) and a local MCP server (stdio) for AI agent queries.

v1 is deliberately the smallest end-to-end path: CSV upload, serial categorization, DynamoDB write. The worker pool is step 2; receipt image extraction is backlog.

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
  ├─ parser.DetectSchema()       Claude reads first N rows, returns column mapping
  ├─ parser.ParseCSV()           pure Go CSV parsing using the detected schema
  ├─ for each batch of ~50       serial loop in v1, worker pool in step 2
  │     └─ categorizer.Categorize(batch)   one Claude Haiku call per batch
  │           returns: category and confidence per transaction
  └─ storage.SaveExpenses()      batch write to DynamoDB
```

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

Interfaces are defined in the package that uses them, not the package that implements them — keeps implementations swappable without touching call sites.

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
```

**Planned implementations:**

| Interface | Phase 1 | Phase 2 | Phase 3 |
|-----------|---------|---------|---------|
| `Categorizer` | `AnthropicCategorizer` (Haiku) | `BedrockCategorizer` (Llama/Mistral) | `VLLMCategorizer` (on-demand GPU) |
| `CategoryProvider` | `YAMLProvider` | `PostgresProvider` (RDS + RDS Proxy) | — |

Image extraction is intentionally absent from `Categorizer`. It comes back as
its own method when the S3 bucket and the Claude Vision call are built together
— an interface method with no implementation stops every implementation from
satisfying the interface.

---

## Data Model

### DynamoDB — `expenses` table

Single-table design. Access patterns drive the key structure.

**Base table**

| Key | Type | Value |
|-----|------|-------|
| `PK` | Partition key | `EXPENSES` |
| `SK` | Sort key | `<YYYY-MM-DD>#<uuid>` — enables date range queries |

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
| `GSI1SK` | `<YYYY-MM-DD>#<uuid>` |

Enables: "show all grocery expenses in April", "sum transport costs for Q1".

---

## API Endpoints

All endpoints require `X-API-Key` header.

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/uploads/csv` | Upload bank export CSV |
| `GET` | `/expenses` | Query expenses (`from`, `to`, `category`, `limit` params) |
| `GET` | `/expenses/summary` | Spending totals grouped by category (`from`, `to` params) |
| `GET` | `/categories` | List configured categories |

---

## Worker Pool Design (step 2, not in v1)

The plan for parallelizing the Claude API calls in CSV processing (I/O-bound).
v1 walks the batches in a serial loop instead — the first upload is also the
first time handler, parser, categorizer and storage run together, and a serial
loop is far easier to debug. The pool replaces the loop once that path works.

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

## Later Changes (for reference)

0. **Worker pool:** replace the serial batch loop with a bounded pool of goroutines fed by a channel, results collected before the DynamoDB write. First step after v1 runs end to end.
1. **Receipt / bill images:** `POST /uploads/image` stores to S3 (30-day lifecycle) and a Vision call extracts `[]Transaction`, which then takes the same path as CSV rows. Adds the S3 bucket to Terraform and a method back onto `Categorizer`.
2. **SQS fan-out:** S3 event on CSV upload triggers SQS. Lambda reads SQS batches instead of the whole file. Worker pool moves from intra-Lambda goroutines to parallel Lambda invocations.
3. **RDS PostgreSQL:** `CategoryProvider` swaps from YAML to Postgres. Adds `categories`, `vendor_rules`, and `budgets` tables. Lambda gets RDS Proxy for connection pooling. Requires VPC in Terraform.
4. **Bedrock:** `Categorizer` swaps from Anthropic SDK to `BedrockCategorizer` using `InvokeModelWithResponseStream`. Model ID configurable via env var, no changes in callers.
