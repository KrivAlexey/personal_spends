# personal_spends

[![CI](https://github.com/KrivAlexey/personal_spends/actions/workflows/ci.yml/badge.svg)](https://github.com/KrivAlexey/personal_spends/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/KrivAlexey/personal_spends/branch/main/graph/badge.svg)](https://codecov.io/gh/KrivAlexey/personal_spends)

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

## Environment Setup

1. **Install tooling** — Go 1.22+, [AWS CLI v2](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html), [Terraform 1.6+](https://developer.hashicorp.com/terraform/install).

2. **Create an IAM user for Terraform** — don't use your AWS root account or its access keys. Create a dedicated IAM user, then attach a policy granting only what's needed to manage the resources in `terraform/main.tf`. The current required permissions are documented in [`terraform/iam-policy.json`](terraform/iam-policy.json) — paste it into the IAM console's JSON policy editor (replace `<ACCOUNT_ID>` with your AWS account ID). Update this file whenever `main.tf` gains new resource types.

3. **Configure AWS credentials**:
   ```bash
   aws configure
   ```
   Use the access key/secret for the IAM user above. Region: `eu-central-1`.

4. **Anthropic API key** — create `.env`:
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
