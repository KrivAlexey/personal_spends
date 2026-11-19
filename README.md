# personal_spends

[![CI](https://github.com/KrivAlexey/personal_spends/actions/workflows/ci.yml/badge.svg)](https://github.com/KrivAlexey/personal_spends/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/KrivAlexey/personal_spends/branch/main/graph/badge.svg)](https://codecov.io/gh/KrivAlexey/personal_spends)

Personal expense tracking with AI categorization. Upload a bank CSV export; Claude categorizes each transaction; results land in DynamoDB and are queryable through a REST API and a local MCP server for AI agents.

**Status:** work in progress. The core packages (CSV parsing, categorization, categories, DynamoDB storage) and the CSV upload handler are built and tested. The local HTTP server (`cmd/server`) runs it end to end. The other HTTP handlers, the Lambda and MCP entry points, the MCP server and the Lambda / API Gateway infrastructure are not built yet. See [Implementation Status](docs/architecture.md#implementation-status).

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

## Run locally

```bash
set -a; . ./.env; set +a     # the server reads env vars, it doesn't load .env itself
go run ./cmd/server          # from the repo root; serves localhost:8080
curl -X POST 'localhost:8080/uploads/csv?bank=sparkasse' --data-binary @export.csv
```

An upload calls Claude and writes to the DynamoDB table (`DYNAMODB_TABLE`, default `personal-spends-expenses`), so it costs money and needs the table from `terraform apply`. Other settings are listed in [`docs/architecture.md`](docs/architecture.md#configuration).

### Offline: no Anthropic, no AWS

Run [DynamoDB Local](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/DynamoDBLocal.html) on port 8000 (Docker `amazon/dynamodb-local`, or the jar with Java 17+), then create the table in it. The Terraform workspace `local` targets `localhost:8000` with fake credentials and keeps its own state, apart from the real one:

```bash
docker run -d -p 8000:8000 amazon/dynamodb-local -jar DynamoDBLocal.jar -inMemory
cd terraform && terraform workspace new local   # once; later: terraform workspace select local
terraform apply
terraform workspace select default              # back to real AWS
```

With `-inMemory` the table is gone when the emulator stops; run `terraform apply` in the `local` workspace again after a restart (delete `terraform.tfstate.d/local/` first if it still lists the table).

Then start the server with the keyword categorizer (no model, no API key) and the emulator endpoint, and upload the fake export:

```bash
CATEGORIZER=fake AWS_ENDPOINT_URL_DYNAMODB=http://localhost:8000 AWS_ACCESS_KEY_ID=local AWS_SECRET_ACCESS_KEY=local go run ./cmd/server
curl -X POST 'localhost:8080/uploads/csv?bank=sparkasse' --data-binary @samples/sparkasse_fake.csv
aws dynamodb scan --table-name personal-spends-expenses --endpoint-url http://localhost:8000
```

The first upload imports 5 rows and skips 1 pending; a second upload reports them as `already_known`.

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
