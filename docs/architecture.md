# Architecture

## Overview

`personal_spends` is a Go backend that accepts bank CSV exports and receipt images, categorizes expenses using Claude AI, and stores structured results in DynamoDB. It exposes a REST API (API Gateway + Lambda) and a local MCP server (stdio) for AI agent queries.

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
│                                          │   ├─ worker pool │   │
│                                          │   ├─ categorizer─┼───┼──► Anthropic API
│                                          │   └─ storage     │   │    (Claude Haiku)
│                                          │        │    │    │   │
│                                          │        ▼    ▼    │   │
│                                          │  DynamoDB   S3   │   │
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
  ├─ worker pool                 fan out batches of ~50 transactions to goroutines
  │     goroutine × N
  │       └─ categorizer.Categorize(batch)   one Claude Haiku call per batch
  │             returns: category, confidence, notes per transaction
  └─ storage.SaveExpenses()      batch write to DynamoDB
```

### Receipt / bill image upload

```
POST /uploads/image  (multipart, field: "file")
  │
  ├─ S3 PutObject                store image (30-day lifecycle policy)
  ├─ categorizer.ExtractFromImage()   Claude Vision returns []Transaction
  ├─ categorizer.Categorize()         categorize extracted transactions
  └─ storage.SaveExpenses()
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
    ExtractFromImage(ctx context.Context, imageURL string) ([]Transaction, error)
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
| `source` | String | `csv` or `image` |
| `created_at` | String | ISO 8601 |

**GSI1 — category queries**

| Key | Value |
|-----|-------|
| `GSI1PK` | `CAT#<category>` e.g. `CAT#groceries` |
| `GSI1SK` | `<YYYY-MM-DD>#<uuid>` |

Enables: "show all grocery expenses in April", "sum transport costs for Q1".

### S3 — `personal-spends-receipts`

Raw receipt and bill images. 30-day lifecycle policy configured in Terraform.

---

## API Endpoints

All endpoints require `X-API-Key` header.

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/uploads/csv` | Upload bank export CSV |
| `POST` | `/uploads/image` | Upload receipt or bill image |
| `GET` | `/expenses` | Query expenses (`from`, `to`, `category`, `limit` params) |
| `GET` | `/expenses/summary` | Spending totals grouped by category (`from`, `to` params) |
| `GET` | `/categories` | List configured categories |

---

## Worker Pool Design

Used for CSV processing to parallelize Claude API calls (I/O-bound).

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
| `S3_BUCKET` | Receipt image bucket | `personal-spends-receipts` |
| `AWS_REGION` | AWS region | `eu-central-1` |
| `WORKER_POOL_SIZE` | Max concurrent categorizer goroutines | `5` |
| `BATCH_SIZE` | Transactions per Claude API call | `50` |

---

## Phase 2 Changes (for reference)

1. **SQS fan-out:** S3 event on CSV upload triggers SQS. Lambda reads SQS batches instead of the whole file. Worker pool moves from intra-Lambda goroutines to parallel Lambda invocations.
2. **RDS PostgreSQL:** `CategoryProvider` swaps from YAML to Postgres. Adds `categories`, `vendor_rules`, and `budgets` tables. Lambda gets RDS Proxy for connection pooling. Requires VPC in Terraform.
3. **Bedrock:** `Categorizer` swaps from Anthropic SDK to `BedrockCategorizer` using `InvokeModelWithResponseStream`. Model ID configurable via env var, no changes in callers.
