# Design

## Context

- `compose.yaml` runs DynamoDB Local (`-inMemory`), Postgres (`tmpfs` data dir) and S3Mock (creates `personal-spends-uploads` at startup). None of them keeps data on disk.
- The DynamoDB table comes from `terraform apply` in the `local` workspace, whose provider endpoint is tied to `terraform.workspace == "local"` (`terraform/main.tf`). Its state lives in `terraform/terraform.tfstate.d/local/` and goes stale whenever the containers restart.
- The server creates the Postgres `uploads` table itself at startup (`EnsureSchema`); the S3 bucket comes from S3Mock.
- `scripts/run-local.sh` already uses a `curl` probe on `localhost:8080` to refuse a busy port.

## Goals / Non-Goals

**Goals:** one idempotent command, safe to run on a fresh clone or a stack that is down, up or half-broken; never able to reach real AWS.

**Non-Goals:** emptying data item by item; any real-AWS cleanup; starting the server.

## Decisions

1. **Wipe by recreating containers, not by deleting rows.** `docker compose down` then `up -d` drops everything, since all three services are in memory. Alternative: `DELETE`/scan-and-delete per store. That means three clients and code that rots as stores are added, for the same result.
2. **`TF_WORKSPACE=local` instead of `terraform workspace select`.** The env var scopes the workspace to the script's own commands, so the owner's saved selection (normally `default`, real AWS) is never changed and an interrupted script can't leave the next manual `terraform apply` pointed at the wrong state. Because the emulator endpoint follows the workspace name, `local` can only ever reach `localhost:8000`.
3. **Delete the state files, keep the workspace.** Remove `terraform.tfstate.d/local/terraform.tfstate*`, then `terraform apply -auto-approve`. On a fresh clone where the `local` workspace doesn't exist yet, create it with `terraform workspace new local` and immediately `terraform workspace select default`. `-auto-approve` is acceptable here only because the workspace is pinned to the emulator.
4. **Wait for DynamoDB Local before applying.** Poll `http://localhost:8000` with `curl` for up to ~30 s; compose `--wait` only knows the container started, not that the port answers (the services define no healthchecks).
5. **Refuse when the server is running.** Reuse run-local.sh's `curl -s --max-time 2 localhost:8080` probe: a server kept across the reset would fail on the missing Postgres table. Stopping it automatically is out of scope.

## Risks / Trade-offs

- [`terraform init` not yet run in `terraform/`] → the script runs `terraform init -input=false` first (no-op once initialised).
- [Container names or ports change in `compose.yaml`] → the script calls `docker compose` by service, not container name; only the `8000` probe and the `8080` check hard-code ports, the same ones README documents.
- [Someone points `AWS_ENDPOINT_URL_DYNAMODB` elsewhere] → irrelevant: Terraform's endpoint comes from `main.tf`, not the env, and the script never calls the AWS CLI.
