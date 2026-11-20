# 0017 — RDS PostgreSQL for the uploads table, built with the Lambda

**Status:** Accepted

## Context

`cmd/server` records every upload in a PostgreSQL `uploads` table ([0015](0015-upload-files-and-records.md)) and won't start without `DATABASE_URL`. That Postgres exists only in the local compose stack. Terraform creates the DynamoDB table and the uploads S3 bucket, but no database, so the AWS deployment (`cmd/lambda` behind API Gateway, [0001](0001-lambda-over-ecs.md)) would have nowhere to record uploads (#129).

`cmd/lambda` isn't built yet, so nothing in AWS would use a database today.

Backlog item 4 already planned RDS PostgreSQL for categories, vendor rules and budgets, with RDS Proxy and a VPC.

## Decision

- **The `uploads` table runs on Amazon RDS for PostgreSQL in AWS**, as backlog item 4 planned. Later, the same database also takes the backlog's `categories`, `vendor_rules` and `budgets` tables.
- **It's built together with `cmd/lambda` and API Gateway**, the first deployment that needs it, not before. Until then `uploads` runs only locally and RDS costs nothing.
- **Planned shape**, to be settled in that change's Terraform and plan output:
  - **Instance:** a single-AZ PostgreSQL instance on the smallest burstable instance class, gp3 storage. It's personal data with one user, so Multi-AZ isn't worth paying for.
  - **Network:** a VPC with private subnets in two availability zones (an RDS subnet group needs two). The instance is not publicly accessible, and security groups allow port 5432 only from the Lambda and the proxy.
  - **Connections:** RDS Proxy between the Lambda and the database, so concurrent invocations share pooled connections instead of each opening its own.
  - **Credentials:** the master password is managed by RDS in Secrets Manager. The proxy reads it from there. Nothing goes in Terraform state or environment variables in plain text.
  - **Lambda outbound traffic:** the Lambda runs in the VPC's private subnets. It reaches S3 and DynamoDB through gateway VPC endpoints.
- **Schema:** still `EnsureSchema` at startup ([0015](0015-upload-files-and-records.md)) until a migration tool is needed.
- **Applying is the owner's call:** `terraform apply` for any of this waits for the owner's approval (CLAUDE.md), because RDS bills by the hour from the moment it exists.

## Open question for the Lambda change

**How the Lambda reaches the Anthropic API from inside the VPC.** A NAT gateway is simplest, but it bills by the hour and is likely to cost more than the database. Alternatives include a NAT instance, or running the categorizer outside the VPC. Choose with current prices when that change is planned.

## Alternatives considered

| Option | Rejected because |
|---|---|
| Aurora Serverless v2 (PostgreSQL) | Still needs the same VPC work; scaling down when idle mostly pays off under a variable load, and this one is tiny and steady |
| Hosted Postgres outside AWS (Neon, Supabase) | No VPC and cheap, but skips the VPC, RDS and RDS Proxy work this project is meant to teach, and puts bank-statement metadata with another provider |
| Keep `uploads` in DynamoDB instead (a conditional write for the claim) | No new infrastructure, but drops the SQL practice the owner chose in 0015, and categories and budgets need Postgres later anyway |
| RDS publicly accessible, Lambda outside the VPC | Avoids the NAT question, but puts the database on the internet behind only a password |
| Build RDS now, before the Lambda | Pays for an idle database with nothing in AWS using it |

## Consequences

- Until `cmd/lambda` is built, uploads work only locally. Backlog item 4 moves forward: its network, instance and proxy parts come with the Lambda work. Its categories and budgets parts stay where they are.
- The Lambda change becomes the project's first VPC work: subnets, route tables, security groups, VPC endpoints and the NAT question. It's also where the flat-Terraform choice ([0006](0006-flat-terraform-first.md)) gets tested by a much larger `main.tf`.
- Local development is unchanged: the compose Postgres stays.
- Monthly cost starts at the first `terraform apply` of the database, the proxy and any NAT. The Lambda change's PR states the expected cost from current pricing before anything is applied.
