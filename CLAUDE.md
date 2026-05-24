# personal_spends

Go backend for personal expense tracking and AI categorization. Accepts bank export CSVs and receipt/bill images, categorizes expenses using Claude AI, stores results in DynamoDB, and exposes an MCP server so AI agents can query spending data.

**Learning goals for this project:** Go concurrency patterns, Claude API (vision + tool use), AWS Lambda, Terraform, MCP server implementation, agentic development practices.

## Architecture

```
CSV upload    ──► parser ──► worker pool (goroutines + channels) ──► Categorizer ──► DynamoDB
Image upload  ──► S3 ──► Claude Vision (extract items) ──────────► Categorizer ──► DynamoDB

Interfaces:
  REST API    — upload files, query expenses, get summaries (API Gateway + Lambda)
  MCP server  — AI agents query expenses via tools (local stdio binary)
```

**AWS services:** Lambda (compute), API Gateway (REST), DynamoDB (expenses storage), S3 (receipt image uploads)
**Infrastructure:** Terraform, flat `main.tf` for v1

### Entry points

Three thin `main.go` files, all importing the same `internal/` packages:

```
cmd/lambda/    — production Lambda handler (deployed to AWS)
cmd/server/    — local HTTP server for development (localhost:8080)
cmd/mcp/       — MCP server binary, runs locally via stdio
```

## Project Structure

```
cmd/
  lambda/         Lambda entry point
  server/         Local dev HTTP server
  mcp/            MCP server (stdio transport)
internal/
  handler/        HTTP/Lambda request handlers
  categorizer/    Categorizer interface + Anthropic Haiku implementation
  parser/         CSV parsing and schema detection
  storage/        DynamoDB read/write
  categories/     CategoryProvider interface + YAML implementation
  mcp/            MCP tool definitions and handlers
terraform/        Flat main.tf — Lambda, API Gateway, DynamoDB, S3
docs/
  architecture.md Detailed architecture and data model
  decisions/      Architecture Decision Records (ADRs)
samples/          Test fixtures — gitignored for real data, use fake data only
```

## Development

### Prerequisites

- Go 1.22+
- AWS CLI configured (`aws configure`)
- Terraform 1.6+
- Anthropic API key in `.env`: `ANTHROPIC_API_KEY=sk-ant-...`

### Run locally

```bash
go run ./cmd/server        # HTTP server on localhost:8080
go test ./...              # all tests
go build ./...             # build check
```

### Deploy

```bash
cd terraform
terraform init
terraform plan
terraform apply
```

## Code Conventions

- Standard Go project layout: `cmd/` for entry points, `internal/` for all shared logic
- Errors returned, not panicked — `fmt.Errorf("context: %w", err)`
- `context.Context` as the first argument in every function that does I/O
- No global state — dependencies injected via structs
- Table-driven tests in `_test.go` files alongside the code they test
- Interfaces defined in the package that uses them, not the package that implements them

## Key Design Decisions

- **Lambda over ECS:** personal-scale workload, scales to zero, free tier covers normal use, simpler Terraform
- **DynamoDB for expenses:** no VPC complexity, fits Lambda's stateless model, sufficient for date and category queries
- **Worker pool for CSV processing:** fan out N batches concurrently via goroutines and channels; batch size ~50 transactions per Claude API call
- **Categorizer as interface from day one:** swappable between Anthropic API, AWS Bedrock, and future on-demand GPU without changing callers
- **CategoryProvider as interface from day one:** YAML file for v1, PostgreSQL RDS (with RDS Proxy) for Phase 2
- **MCP server is local, not in Lambda:** Lambda cold starts would make MCP calls feel laggy; local binary calls the REST API as a client
- **Claude for CSV schema detection:** Claude reads the first few rows to detect column layout once, result cached; pure Go parses subsequent rows with that schema
- **Flat Terraform first:** single `main.tf` to learn the basics; refactor into modules as a dedicated exercise later
- **EUR only for v1:** simplifies storage and display; multi-currency added if needed

See `docs/decisions/` for full ADRs.

## Phase 2 Backlog (in order)

1. SQS fan-out — split large CSV uploads into SQS messages, parallel Lambda invocations per batch
2. PostgreSQL RDS (Aurora Serverless v2 + RDS Proxy) — categories, vendor rules, budgets schema
3. AWS Bedrock — swap Categorizer implementation to model-agnostic Bedrock InvokeModel
4. MCP SSE transport — make the MCP server reachable from claude.ai and other remote agents
5. On-demand GPU — vLLM on spot EC2, brought up only during processing, for self-hosted model experiments

## MCP Tools (v1)

| Tool | Description |
|------|-------------|
| `get_expenses` | Query expenses by date range, category, or amount |
| `get_summary` | Spending totals grouped by category for a period |
| `list_categories` | List all configured categories |
| `add_expense` | Manually add a single expense record |

## AI Collaboration Notes

### Model selection

Default to **Sonnet 4.6** for this project. Switch to **Opus 4.7** when the work needs cross-system reasoning or deep trade-off analysis.

**Use Sonnet for:**
- Implementation, code generation, writing tests
- Refactoring within a single component
- Terraform and YAML boilerplate
- Documentation and ADR updates
- Routine debugging

**Switch to Opus for:**
- Cross-subsystem architecture changes (e.g., changing the Categorizer or CategoryProvider interfaces)
- Trade-off analysis spanning Go + AWS + cost + maintainability
- Hard bugs where multiple components could be at fault
- Designing new abstractions or refactoring across layers

**Claude Code:** when you notice the conversation is heading into one of the Opus-worthy areas, suggest the user runs `/model` to switch. It costs more tokens overall to fix a wrong architectural choice later than to spend Opus tokens reasoning through it correctly the first time.
