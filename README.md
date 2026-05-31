# personal_spends

Personal expense tracking with AI categorization. Accepts bank export CSVs and receipt/bill images, categorizes expenses using Claude AI, stores results in DynamoDB, and exposes an MCP server so AI agents can query spending data.

## Architecture

```
CSV upload    ──► parser ──► worker pool (goroutines + channels) ──► Categorizer ──► DynamoDB
Image upload  ──► S3 ──► Claude Vision (extract items) ──────────► Categorizer ──► DynamoDB

Interfaces:
  REST API    — upload files, query expenses, get summaries (API Gateway + Lambda)
  MCP server  — AI agents query expenses via tools (local stdio binary)
```

**AWS services:** Lambda, API Gateway, DynamoDB, S3  
**Infrastructure:** Terraform (`terraform/main.tf`)

## Project Structure

```
cmd/
  lambda/       Lambda entry point (production)
  server/       Local HTTP server (development)
  mcp/          MCP server binary (stdio transport)
internal/
  handler/      HTTP/Lambda request handlers
  categorizer/  Categorizer interface + Anthropic Claude implementation
  parser/       CSV parsing and schema detection
  storage/      DynamoDB read/write
  categories/   CategoryProvider interface + YAML implementation
  mcp/          MCP tool definitions and handlers
terraform/      Lambda, API Gateway, DynamoDB, S3
docs/
  architecture.md   Data model and component design
  decisions/        Architecture Decision Records (ADRs)
samples/        Test fixtures (gitignored for real data — use fake data only)
```

## Prerequisites

- Go 1.22+
- AWS CLI configured (`aws configure`)
- Terraform 1.6+
- Anthropic API key in `.env`:
  ```
  ANTHROPIC_API_KEY=sk-ant-...
  ```

## Run locally

```bash
go run ./cmd/server        # HTTP server on localhost:8080
go test ./...              # all tests
go build ./...             # build check
```

## Deploy

```bash
cd terraform
terraform init
terraform plan
terraform apply
```

## MCP Tools

| Tool | Description |
|------|-------------|
| `get_expenses` | Query expenses by date range, category, or amount |
| `get_summary` | Spending totals grouped by category for a period |
| `list_categories` | List all configured categories |
| `add_expense` | Manually add a single expense record |

## Notes

- EUR only (v1)
- Categories defined in `categories.yaml`
- MCP server runs locally and calls the REST API — not deployed to Lambda
