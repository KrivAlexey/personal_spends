# personal_spends

[![CI](https://github.com/KrivAlexey/personal_spends/actions/workflows/ci.yml/badge.svg)](https://github.com/KrivAlexey/personal_spends/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/KrivAlexey/personal_spends/branch/main/graph/badge.svg)](https://codecov.io/gh/KrivAlexey/personal_spends)

Personal expense tracking with AI categorization. Upload a bank CSV export; Claude categorizes each transaction; results land in DynamoDB and are queryable through a REST API and a local MCP server for AI agents.

**Status:** work in progress. The core packages (CSV parsing, categorization, categories, DynamoDB storage, upload records in PostgreSQL with the files in S3) and the CSV upload handler are built and tested. The local HTTP server (`cmd/server`) runs it end to end. The other HTTP handlers, the Lambda and MCP entry points, the MCP server and the Lambda / API Gateway infrastructure are not built yet. See [Implementation Status](docs/architecture.md#implementation-status).

## Documentation

- [`docs/architecture.md`](docs/architecture.md): design, data model, API, MCP tools, configuration, backlog
- [`docs/decisions/`](docs/decisions/): Architecture Decision Records
- [`openspec/specs/`](openspec/specs/): behavior specs
- [`CLAUDE.md`](CLAUDE.md): conventions and workflow for AI-assisted development

## Setup

1. **Install tooling:** Go 1.26+, [AWS CLI v2](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html), [Terraform 1.6+](https://developer.hashicorp.com/terraform/install), Docker with the [Compose plugin](https://docs.docker.com/compose/install/linux/) (for the local Postgres, S3 and DynamoDB).

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

Or open http://localhost:8080/ in a browser: it uploads an export and lists every upload with its status and counts ([screenshots](docs/screenshots/web-ui/)).

The server also needs `DATABASE_URL` (PostgreSQL, for upload records) and `UPLOADS_BUCKET` (S3, for the uploaded files). PostgreSQL has no AWS deployment yet, so use the compose one below. An upload calls Claude and writes to the DynamoDB table (`DYNAMODB_TABLE`, default `personal-spends-expenses`), so it costs money and needs the table from `terraform apply`. Posting the same bytes again returns the first upload's `upload_id` and counts without importing anything ([0015](docs/decisions/0015-upload-files-and-records.md)). Other settings are listed in [`docs/architecture.md`](docs/architecture.md#configuration). The API's OpenAPI document is served at `localhost:8080/openapi.yaml`.

### Offline: no Anthropic, no AWS

`compose.yaml` starts [DynamoDB Local](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/DynamoDBLocal.html) on port 8000, PostgreSQL on 5432 (user, password and database `spends`) and [S3Mock](https://github.com/adobe/S3Mock) on 9090 with the bucket `personal-spends-uploads`, all in memory. Then create the DynamoDB table: the Terraform workspace `local` targets `localhost:8000` with fake credentials and keeps its own state, apart from the real one. The server creates the Postgres table itself.

```bash
docker compose up -d
cd terraform && terraform workspace new local   # once; later: terraform workspace select local
terraform apply
terraform workspace select default              # back to real AWS
```

The containers keep nothing: the table is gone when DynamoDB Local stops. To get back to a clean, empty stack, stop the server and run `scripts/reset-local.sh`: it recreates the containers (wiping DynamoDB, Postgres and S3), deletes the stale `local` Terraform state and applies the table again. It creates the `local` workspace if needed, leaves your selected workspace alone, never reaches real AWS, and refuses to run while something answers on `localhost:8080`.

Then start the server with the keyword categorizer (no model, no API key) and the emulator endpoints, and upload the fake export:

```bash
CATEGORIZER=fake AWS_ACCESS_KEY_ID=local AWS_SECRET_ACCESS_KEY=local \
  AWS_ENDPOINT_URL_DYNAMODB=http://localhost:8000 AWS_ENDPOINT_URL_S3=http://localhost:9090 \
  UPLOADS_BUCKET=personal-spends-uploads DATABASE_URL=postgres://spends:spends@localhost:5432/spends \
  go run ./cmd/server
curl -X POST 'localhost:8080/uploads/csv?bank=sparkasse&filename=sparkasse_fake.csv' --data-binary @samples/sparkasse_fake.csv
aws dynamodb scan --table-name personal-spends-expenses --endpoint-url http://localhost:8000
docker compose exec postgres psql -U spends -c 'SELECT * FROM uploads'
```

Or open http://localhost:8080/, pick `sparkasse` and upload `samples/sparkasse_fake.csv`. The first upload imports 5 rows and skips 1 pending. Posting the same file again returns the same `upload_id` and counts without importing anything. A changed file whose rows overlap reports the overlapping rows as `already_known`.

To skip typing the variables, put them in `.env.local` (gitignored) and run `scripts/run-local.sh`, which also opens http://localhost:8080/ in your browser (`xdg-open`, or prints the URL) once the server answers, and refuses to start if something already listens on that port; pass another env file as the first argument, e.g. `scripts/run-local.sh .env`. The file is sourced as shell code, so quote values that contain spaces.

## Build and test

```bash
go build ./... && go vet ./... && go test ./...
```

The live Claude API test runs only when `ANTHROPIC_API_KEY` is set. The PostgreSQL and S3 integration tests run only against the compose containers:

```bash
TEST_DATABASE_URL=postgres://spends:spends@localhost:5432/spends TEST_S3_ENDPOINT=http://localhost:9090 go test ./internal/uploads
```

They empty the `uploads` table, so don't point `TEST_DATABASE_URL` at a database you want to keep.

## Deploy

```bash
cd terraform
terraform init
terraform plan
terraform apply
```

Currently this creates the DynamoDB table and the uploads S3 bucket (its name is the `uploads_bucket_name` output, for `UPLOADS_BUCKET`). PostgreSQL isn't deployed yet: it comes as RDS with the Lambda deployment ([0017](docs/decisions/0017-rds-postgres-for-uploads.md)).
