# personal_spends

[![CI](https://github.com/KrivAlexey/personal_spends/actions/workflows/ci.yml/badge.svg)](https://github.com/KrivAlexey/personal_spends/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/KrivAlexey/personal_spends/branch/main/graph/badge.svg)](https://codecov.io/gh/KrivAlexey/personal_spends)

Personal expense tracking with AI categorization. Upload a bank CSV export; Claude categorizes each transaction; results land in DynamoDB and are queryable through a REST API and a local MCP server for AI agents.

**Status:** work in progress. The core packages (CSV parsing, categorization, categories, DynamoDB storage) are built and tested. The HTTP handlers, entry points, MCP server and the Lambda / API Gateway infrastructure are not built yet. See [Implementation Status](docs/architecture.md#implementation-status).

## Documentation

- [`docs/architecture.md`](docs/architecture.md): design, data model, API, MCP tools, configuration, backlog
- [`docs/decisions/`](docs/decisions/): Architecture Decision Records
- [`openspec/specs/`](openspec/specs/): behavior specs
- [`CLAUDE.md`](CLAUDE.md): conventions and workflow for AI-assisted development

## Setup

1. **Install tooling:** Go 1.26+, [AWS CLI v2](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html), [Terraform 1.6+](https://developer.hashicorp.com/terraform/install).

2. **Create an IAM user for Terraform.** Don't use your AWS root account or its access keys. Create a dedicated IAM user and attach only the permissions needed for the resources in `terraform/main.tf`. They're listed in [`terraform/iam-policy.json`](terraform/iam-policy.json): paste it into the IAM console's JSON policy editor, replacing `<ACCOUNT_ID>` with your AWS account ID. Update the file whenever `main.tf` gains new resource types.

3. **Configure AWS credentials** for that user, region `eu-central-1`:
   ```bash
   aws configure
   ```

4. **Anthropic API key:** create `.env` (gitignored):
   ```
   ANTHROPIC_API_KEY=sk-ant-...
   ```

## Build and test

```bash
go build ./... && go vet ./... && go test ./...
```

The live Claude API test runs only when `ANTHROPIC_API_KEY` is set.

## Deploy

```bash
cd terraform
terraform init
terraform plan
terraform apply
```

Currently this creates only the DynamoDB table.
